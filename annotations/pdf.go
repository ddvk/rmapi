package annotations

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/encoding/rm"
	"github.com/juruen/rmapi/log"
	"github.com/unidoc/unipdf/v3/annotator"
	"github.com/unidoc/unipdf/v3/contentstream"
	"github.com/unidoc/unipdf/v3/contentstream/draw"
	"github.com/unidoc/unipdf/v3/core"
	"github.com/unidoc/unipdf/v3/creator"
	pdf "github.com/unidoc/unipdf/v3/model"
)

const (
	DeviceWidth  = 1404
	DeviceHeight = 1872
)

var rmPageSize = creator.PageSize{445, 594}

type PdfGenerator struct {
	zipName        string
	outputFilePath string
	options        PdfGeneratorOptions
	pdfReader      *pdf.PdfReader
	template       bool
}

type PdfGeneratorOptions struct {
	AddPageNumbers  bool
	AllPages        bool
	AnnotationsOnly bool //export the annotations without the background/pdf
}

func CreatePdfGenerator(zipName, outputFilePath string, options PdfGeneratorOptions) *PdfGenerator {
	return &PdfGenerator{zipName: zipName, outputFilePath: outputFilePath, options: options}
}

func normalized(p1 rm.Point, scale, xShift float64) (float64, float64) {
	return float64(p1.X)*scale + xShift, float64(p1.Y) * scale
}

func (p *PdfGenerator) Generate() error {
	file, err := os.Open(p.zipName)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	zip := archive.NewZip()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	err = zip.Read(file, fi.Size())
	if err != nil {
		return err
	}

	if zip.Content.FileType == "epub" {
		return errors.New("only pdf and notebooks supported")
	}

	if err = p.initBackgroundPages(zip.Payload); err != nil {
		return err
	}

	if len(zip.Pages) == 0 {
		return errors.New("the document has no pages")
	}

	c := creator.New()
	if p.template {
		// use the standard page size
		c.SetPageSize(rmPageSize)
	}

	if p.pdfReader != nil && p.options.AllPages {
		outlines := p.pdfReader.GetOutlineTree()
		c.SetOutlineTree(outlines)
	}

	for _, pageAnnotations := range zip.Pages {
		hasContent := pageAnnotations.Data != nil

		// For PDFs, always include all pages so the full document is
		// preserved with annotations overlaid. For notebooks (no source
		// PDF), skip blank pages unless -a is given.
		if !hasContent && !p.options.AllPages && p.pdfReader == nil {
			continue
		}
		//1 based, redirected page
		pageNum := pageAnnotations.DocPage + 1

		page, err := p.addBackgroundPage(c, pageNum)
		if err != nil {
			return err
		}

		if page == nil {
			log.Error.Fatal("page is null")
		}
		if !hasContent {
			continue
		}

		var scale float64
		var xShift float64
		isV6Pdf := pageAnnotations.Data.Version == rm.V6 && pageAnnotations.DocPage >= 0

		if isV6Pdf {
			// v6 PDF-backed pages: DPI-based scale (226 DPI device → 72 DPI PDF).
			// The v6 parser adds +Width/2 to X so that x=Width/2 is page center;
			// xShift compensates so that maps to c.Width()/2 in PDF points.
			scale = 72.0 / 226.0
			xShift = c.Width()/2 - float64(rm.Width)/2*scale
		} else {
			// v3/v5, v6 blank/added pages, or notebooks
			ratio := c.Height() / c.Width()
			if ratio < 1.33 {
				scale = c.Width() / DeviceWidth
			} else {
				scale = c.Height() / DeviceHeight
			}
		}

		contentCreator := contentstream.NewContentCreator()
		contentCreator.Add_q()
		bounds := annotationBounds{}

		for _, layer := range pageAnnotations.Data.Layers {
			for _, line := range layer.Lines {
				if len(line.Points) < 1 {
					continue
				}
				if line.BrushType == rm.Eraser {
					continue
				}

				if line.BrushType == rm.HighlighterV5 {
					last := len(line.Points) - 1
					x1, y1 := normalized(line.Points[0], scale, xShift)
					x2, _ := normalized(line.Points[last], scale, xShift)
					// make horizontal lines only, use y1
					width := scale * 30
					y1 += width / 2

					lineDef := annotator.LineAnnotationDef{X1: x1 - 1, Y1: c.Height() - y1, X2: x2, Y2: c.Height() - y1}
					r, g, b := highlightRGB(line.BrushColor, nil)
					lineDef.LineColor = pdf.NewPdfColorDeviceRGB(r, g, b)
					lineDef.Opacity = 0.5
					lineDef.LineWidth = width
					ann, err := annotator.CreateLineAnnotation(lineDef)
					if err != nil {
						return err
					}
					page.AddAnnotation(ann)
					if err := bounds.includeAnnotation(ann); err != nil {
						return err
					}
				} else {
					path := draw.NewPath()
					for i := 0; i < len(line.Points); i++ {
						x1, y1 := normalized(line.Points[i], scale, xShift)
						path = path.AppendPoint(draw.NewPoint(x1, c.Height()-y1))
					}

					var lineWidth float64
					if pageAnnotations.Data.Version == rm.V6 {
						// v6: per-point Width is in device pixels; average and scale to PDF points.
						var totalW float64
						for _, pt := range line.Points {
							totalW += float64(pt.Width)
						}
						lineWidth = totalW / float64(len(line.Points)) * scale
					} else {
						lineWidth = float64(line.BrushSize*6.0 - 10.8)
					}
					if lineWidth < 0.1 {
						lineWidth = 0.1
					}
					contentCreator.Add_w(lineWidth)
					// Default PDF miter limit is 10; a join may extend up to
					// five stroke widths beyond a point. Include that envelope
					// so thick strokes and sharp joins do not get clipped.
					padding := 5 * lineWidth
					for _, point := range path.Points {
						bounds.include(pdf.PdfRectangle{
							Llx: point.X - padding, Lly: point.Y - padding,
							Urx: point.X + padding, Ury: point.Y + padding,
						})
					}

					switch line.BrushColor {
					case rm.Black:
						contentCreator.Add_rg(1.0, 1.0, 1.0)
					case rm.White:
						contentCreator.Add_rg(0.0, 0.0, 0.0)
					case rm.Grey:
						contentCreator.Add_rg(0.8, 0.8, 0.8)
					case rm.Blue:
						contentCreator.Add_rg(0.0, 0.38, 0.8)
					case rm.Red:
						contentCreator.Add_rg(0.85, 0.03, 0.03)
					}

					//TODO: use bezier
					draw.DrawPathWithCreator(path, contentCreator)

					contentCreator.Add_S()
				}
			}
		}
		for _, hl := range pageAnnotations.Data.Highlights {
			for _, rect := range hl.Rects {
				// Highlight rects don't have the +Width/2 X offset that strokes do;
				// for v6 PDF pages, x=0 is at the horizontal center of the page.
				hlXOff := xShift
				if isV6Pdf {
					hlXOff = c.Width() / 2
				}
				x1 := rect.X*scale + hlXOff
				y1 := c.Height() - rect.Y*scale
				x2 := (rect.X+rect.W)*scale + hlXOff
				y2 := c.Height() - (rect.Y+rect.H)*scale

				lineDef := annotator.LineAnnotationDef{X1: x1, Y1: y2, X2: x2, Y2: y1}
				r, g, b := highlightRGB(hl.Color, hl.ColorRGBA)
				lineDef.LineColor = pdf.NewPdfColorDeviceRGB(r, g, b)
				lineDef.Opacity = 0.3
				lineDef.LineWidth = math.Abs(y1 - y2)
				ann, err := annotator.CreateLineAnnotation(lineDef)
				if err != nil {
					continue
				}
				page.AddAnnotation(ann)
				if err := bounds.includeAnnotation(ann); err != nil {
					return err
				}
			}
		}

		contentCreator.Add_Q()
		drawingOperations := contentCreator.Operations().String()
		pageContentStreams, err := page.GetAllContentStreams()
		//hack: wrap the page content in a context to prevent transformation matrix misalignment
		wrapper := []string{"q", pageContentStreams, "Q", drawingOperations}
		page.SetContentStreams(wrapper, core.NewFlateEncoder())
		// Extend only the page boundaries after rendering. Changing the
		// Creator's dimensions would move or rescale the existing ink and
		// could also affect the following page.
		if err := bounds.expandPage(page); err != nil {
			return err
		}
	}

	return c.WriteToFile(p.outputFilePath)
}

func (p *PdfGenerator) initBackgroundPages(pdfArr []byte) error {
	if len(pdfArr) > 0 {
		pdfReader, err := pdf.NewPdfReader(bytes.NewReader(pdfArr))
		if err != nil {
			return err
		}

		encrypted, err := pdfReader.IsEncrypted()
		if err != nil {
			return nil
		}
		if encrypted {
			valid, err := pdfReader.Decrypt([]byte(""))
			if err != nil {
				return err
			}
			if !valid {
				return fmt.Errorf("cannot decrypt")
			}

		}

		p.pdfReader = pdfReader
		p.template = false
		return nil
	}

	p.template = true
	return nil
}

func (p *PdfGenerator) addBackgroundPage(c *creator.Creator, pageNum int) (*pdf.PdfPage, error) {
	var page *pdf.PdfPage

	// if page == 0 then empty page
	if !p.template && !p.options.AnnotationsOnly && pageNum > 0 {
		tmpPage, err := p.pdfReader.GetPage(pageNum)
		if err != nil {
			return nil, err
		}
		mbox, err := tmpPage.GetMediaBox()
		if err != nil {
			return nil, err
		}

		// TODO: adjust the page if cropped
		pageHeight := mbox.Ury - mbox.Lly
		pageWidth := mbox.Urx - mbox.Llx
		// use the pdf's page size
		c.SetPageSize(creator.PageSize{pageWidth, pageHeight})
		c.AddPage(tmpPage)
		page = tmpPage
	} else {
		page = c.NewPage()
	}

	if p.options.AddPageNumbers {
		c.DrawFooter(func(block *creator.Block, args creator.FooterFunctionArgs) {
			p := c.NewParagraph(fmt.Sprintf("%d", args.PageNum))
			p.SetFontSize(8)
			w := block.Width() - 20
			h := block.Height() - 10
			p.SetPos(w, h)
			block.Draw(p)
		})
	}
	return page, nil
}

// highlightRGB resolves a v6 highlight color to RGB components in [0,1].
// rgba (when non-nil) overrides the enum and carries an explicit color from
// firmware 3.6+. Falls back to yellow for unknown codes.
func highlightRGB(c rm.BrushColor, rgba *[4]uint8) (r, g, b float64) {
	if rgba != nil {
		return float64(rgba[0]) / 255, float64(rgba[1]) / 255, float64(rgba[2]) / 255
	}
	switch c {
	case rm.HighlightYellow, rm.Yellow2:
		return 1.0, 0.93, 0.0
	case rm.HighlightGreen, rm.Green2:
		return 0.36, 0.86, 0.36
	case rm.HighlightPink, rm.Magenta:
		return 1.0, 0.45, 0.75
	case rm.Blue, rm.Cyan:
		return 0.40, 0.75, 1.0
	case rm.Red:
		return 1.0, 0.45, 0.45
	case rm.Black, rm.Grey, rm.GreyOverlap:
		return 0.7, 0.7, 0.7
	}
	return 1.0, 0.93, 0.0
}

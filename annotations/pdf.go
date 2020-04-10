package annotations

import (
	"bytes"
	"fmt"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/encoding/rm"
	"github.com/juruen/rmapi/log"
	annotator "github.com/unidoc/unipdf/v3/annotator"
	"github.com/unidoc/unipdf/v3/contentstream"

	//"github.com/unidoc/unipdf/v3/contentstream/draw/util"
	"os"

	"github.com/unidoc/unipdf/v3/contentstream/draw"
	"github.com/unidoc/unipdf/v3/creator"
	pdf "github.com/unidoc/unipdf/v3/model"
)

const (
	PPI          = 226
	DeviceHeight = 1872
	DeviceWidth  = 1404
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
	AnnotationsOnly bool
}

func CreatePdfGenerator(zipName, outputFilePath string, options PdfGeneratorOptions) *PdfGenerator {
	return &PdfGenerator{zipName: zipName, outputFilePath: outputFilePath, options: options}
}

func normalized(p1 rm.Point, ratioX float64) (float64, float64) {
	return float64(p1.X) * ratioX, float64(p1.Y) * ratioX
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

	if err = p.initBackgroundPages(zip.Pdf); err != nil {
		return err
	}

	c := creator.New()
	if p.template {
		// use the standard page size
		c.SetPageSize(rmPageSize)
	}
	c.SetPageSize(rmPageSize)

	ratioX := c.Width() / DeviceWidth

	for i, pageAnnotations := range zip.Pages {
		hasContent := pageAnnotations.Data != nil

		// do not add a page when there are no annotaions
		if !p.options.AllPages && !hasContent {
			continue
		}

		page, err := p.addBackgroundPage(c, i+1)
		if err != nil {
			return err
		}
		if page == nil {
			log.Error.Fatal("page is null")
		}

		if err != nil {
			return err
		}
		if !hasContent {
			continue
		}

		contentCreator := contentstream.NewContentCreator()
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
					x1, y1 := normalized(line.Points[0], ratioX)
					x2, y2 := normalized(line.Points[last], ratioX)
					// make horizontal lines only
					y2 = y1
					//todo: y cooridnates are reversed
					lineDef := annotator.LineAnnotationDef{X1: x1 - 1, Y1: c.Height() - y1, X2: x2, Y2: c.Height() - y2}
					lineDef.LineColor = pdf.NewPdfColorDeviceRGB(1.0, 1.0, 0.0) //yellow
					lineDef.Opacity = 0.5
					lineDef.LineWidth = 5.0
					ann, err := annotator.CreateLineAnnotation(lineDef)
					if err != nil {
						return err
					}
					page.AddAnnotation(ann)
				} else {
					path := draw.NewPath()
					for i := 0; i < len(line.Points); i++ {
						x1, y1 := normalized(line.Points[i], ratioX)
						path = path.AppendPoint(draw.NewPoint(x1, c.Height()-y1))
					}
					contentCreator.Add_q()
					// fmt.Printf("unk: %f\n", line.Unknown)
					contentCreator.Add_w(float64(line.BrushSize / 1000))
					contentCreator.Add_rg(1.0, 1.0, 0.0)

					draw.DrawPathWithCreator(path, contentCreator)

					contentCreator.Add_S()
					contentCreator.Add_Q()
				}
			}

			ops := contentCreator.Operations()
			bt := ops.Bytes()
			err = page.AppendContentStream(string(bt))
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

		p.pdfReader = pdfReader
		p.template = false
		return nil
	}

	p.template = true
	return nil
}

func (p *PdfGenerator) addBackgroundPage(c *creator.Creator, pageNum int) (*pdf.PdfPage, error) {
	var page *pdf.PdfPage

	if !p.template && !p.options.AnnotationsOnly {
		page1, err := p.pdfReader.GetPage(pageNum)
		if err != nil {
			return nil, err
		}
		block, err := creator.NewBlockFromPage(page1)
		if err != nil {
			return nil, err
		}
		mb, err := page1.GetMediaBox()
		fmt.Println(mb)
		c.SetPageSize(creator.PageSize{block.Width(), block.Height()})
		//convert: Letter->A4
		factor := rmPageSize[0] / block.Width()
		//factor = factor * 0.99
		fmt.Println("Factor", factor)
		//todo: remove hack
		block.SetPos(0.0, 0.0)
		block.Scale(factor, factor)
		page = c.NewPage()

		err = c.Draw(block)
		if err != nil {
			return nil, err
		}
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

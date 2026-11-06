package annotations

import (
	"fmt"
	"math"

	"github.com/unidoc/unipdf/v3/core"
	pdf "github.com/unidoc/unipdf/v3/model"
)

// Bounds are in PDF user-space coordinates, using the original page's
// transform. The page boxes may grow in any direction, including negatives.
type annotationBounds struct {
	rect *pdf.PdfRectangle
}

func unionRect(a, b pdf.PdfRectangle) pdf.PdfRectangle {
	return pdf.PdfRectangle{
		Llx: math.Min(a.Llx, b.Llx), Lly: math.Min(a.Lly, b.Lly),
		Urx: math.Max(a.Urx, b.Urx), Ury: math.Max(a.Ury, b.Ury),
	}
}

func (b *annotationBounds) include(rect pdf.PdfRectangle) {
	if b.rect == nil {
		b.rect = &rect
	} else {
		*b.rect = unionRect(*b.rect, rect)
	}
}

func (b *annotationBounds) includeAnnotation(ann *pdf.PdfAnnotation) error {
	arr, ok := core.GetArray(ann.Rect)
	if !ok {
		return fmt.Errorf("annotation rectangle is not an array")
	}
	rect, err := pdf.NewPdfRectangle(*arr)
	if err != nil {
		return err
	}
	b.include(*rect)
	return nil
}

func (b *annotationBounds) expandPage(page *pdf.PdfPage) error {
	if b.rect == nil {
		return nil
	}
	media, err := page.GetMediaBox()
	if err != nil {
		return err
	}
	crop := page.CropBox
	// CropBox is inheritable. A larger MediaBox alone is insufficient when
	// a page or its parent still clips to the original document rectangle.
	for node := page.Parent; crop == nil && node != nil; {
		dict, ok := core.GetDict(node)
		if !ok {
			return fmt.Errorf("invalid page parent dictionary")
		}
		if obj := dict.Get("CropBox"); obj != nil {
			arr, ok := core.GetArray(obj)
			if !ok {
				return fmt.Errorf("inherited crop box is not an array")
			}
			crop, err = pdf.NewPdfRectangle(*arr)
			if err != nil {
				return err
			}
		}
		node = dict.Get("Parent")
	}
	if expanded := unionRect(*media, *b.rect); expanded != *media {
		// Copy, rather than modifying a box shared by sibling pages.
		page.MediaBox = &expanded
	}
	if crop != nil {
		if expanded := unionRect(*crop, *b.rect); expanded != *crop {
			page.CropBox = &expanded
		}
	}
	return nil
}

package annotations

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/juruen/rmapi/encoding/rm"
	"github.com/unidoc/unipdf/v3/core"
	pdf "github.com/unidoc/unipdf/v3/model"
)

// Build a public, synthetic fixture instead of checking in a user's notes.
// The second page has no ink and must retain its original dimensions.
func generateBoundsFixture(t *testing.T, points []rm.Point, brush rm.BrushType, crop *pdf.PdfRectangle) *pdf.PdfReader {
	t.Helper()
	writer := pdf.NewPdfWriter()
	for i := 0; i < 2; i++ {
		page := pdf.NewPdfPage()
		page.MediaBox = &pdf.PdfRectangle{Llx: 0, Lly: 0, Urx: 600, Ury: 800}
		if crop != nil {
			box := *crop
			page.CropBox = &box
		}
		if err := page.SetContentStreams([]string{"0 0 m 100 100 l S"}, core.NewFlateEncoder()); err != nil {
			t.Fatal(err)
		}
		if err := writer.AddPage(page); err != nil {
			t.Fatal(err)
		}
	}
	var background bytes.Buffer
	if err := writer.Write(&background); err != nil {
		t.Fatal(err)
	}

	// rm.MarshalBinary is a stub; encode the small documented v5 record directly.
	var ink bytes.Buffer
	ink.WriteString(rm.HeaderV5)
	for _, value := range []interface{}{
		uint32(1), uint32(1), brush, rm.Black, uint32(0), rm.Large,
		float32(0), uint32(len(points)), points,
	} {
		if err := binary.Write(&ink, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for name, data := range map[string][]byte{
		"fixture.pdf":      background.Bytes(),
		"fixture.content":  []byte(`{"fileType":"pdf","pageCount":2,"pages":["0","1"]}`),
		"fixture.pagedata": []byte("Blank\nBlank\n"),
		"fixture/0.rm":     ink.Bytes(),
	} {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source, output := filepath.Join(dir, "source.zip"), filepath.Join(dir, "output.pdf")
	if err := os.WriteFile(source, archive.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CreatePdfGenerator(source, output, PdfGeneratorOptions{AllPages: true}).Generate(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := pdf.NewPdfReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func boundsPage(t *testing.T, reader *pdf.PdfReader, n int) *pdf.PdfPage {
	t.Helper()
	p, err := reader.GetPage(n)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExportPreservesInkOutsideAllPageEdges(t *testing.T) {
	// The 600x800 page maps these device coordinates with scale 800/1872.
	points := []rm.Point{{X: -100, Y: -100}, {X: 1700, Y: 2100}}
	r := generateBoundsFixture(t, points, rm.FinelinerV5, nil)
	p := boundsPage(t, r, 1)
	box, err := p.GetMediaBox()
	if err != nil {
		t.Fatal(err)
	}
	scale := 800.0 / DeviceHeight
	pad := float64(rm.Large*6.0-10.8) / 2
	if box.Llx > -100*scale-pad || box.Lly > 800-2100*scale-pad ||
		box.Urx < 1700*scale+pad || box.Ury < 800+100*scale+pad {
		t.Fatalf("export clips out-of-page ink: media box %+v", box)
	}
	streams, err := p.GetAllContentStreams()
	if err != nil {
		t.Fatal(err)
	}
	// Enlarging the page must not rescale or translate the ink/background.
	if !strings.Contains(streams, "0 0 m 100 100 l S") || !strings.Contains(streams, "726.495") {
		t.Fatalf("background or stroke coordinates changed: %s", streams)
	}
	second, err := boundsPage(t, r, 2).GetMediaBox()
	if err != nil {
		t.Fatal(err)
	}
	if *second != (pdf.PdfRectangle{Urx: 600, Ury: 800}) {
		t.Fatalf("blank next page was enlarged: %+v", second)
	}
}

func TestExportExpandsCropBoxWithoutChangingMediaBox(t *testing.T) {
	crop := &pdf.PdfRectangle{Llx: 50, Lly: 50, Urx: 550, Ury: 750}
	r := generateBoundsFixture(t, []rm.Point{{X: 1300, Y: 500}, {X: 1350, Y: 500}}, rm.FinelinerV5, crop)
	p := boundsPage(t, r, 1)
	if p.CropBox == nil || p.CropBox.Urx <= 1350*800.0/DeviceHeight {
		t.Fatalf("crop box still clips ink: %+v", p.CropBox)
	}
	if *p.MediaBox != (pdf.PdfRectangle{Urx: 600, Ury: 800}) {
		t.Fatalf("unnecessary media box change: %+v", p.MediaBox)
	}
	if *boundsPage(t, r, 2).CropBox != *crop {
		t.Fatal("crop box of the unannotated page changed")
	}
}

func TestExportKeepsInPageInkAtOriginalSize(t *testing.T) {
	r := generateBoundsFixture(t, []rm.Point{{X: 200, Y: 200}, {X: 300, Y: 300}}, rm.FinelinerV5, nil)
	p := boundsPage(t, r, 1)
	if *p.MediaBox != (pdf.PdfRectangle{Urx: 600, Ury: 800}) {
		t.Fatalf("in-page ink unexpectedly changed page size: %+v", p.MediaBox)
	}
}

func TestExportIncludesOutOfPageHighlightAppearance(t *testing.T) {
	r := generateBoundsFixture(t, []rm.Point{{X: 1600, Y: 300}, {X: 1800, Y: 300}}, rm.HighlighterV5, nil)
	p := boundsPage(t, r, 1)
	annotations, err := p.GetAnnotations()
	if err != nil || len(annotations) != 1 {
		t.Fatalf("expected one highlight: %v, %v", annotations, err)
	}
	arr, ok := core.GetArray(annotations[0].Rect)
	if !ok {
		t.Fatal("missing highlight rectangle")
	}
	ink, err := pdf.NewPdfRectangle(*arr)
	if err != nil {
		t.Fatal(err)
	}
	box, err := p.GetMediaBox()
	if err != nil {
		t.Fatal(err)
	}
	if box.Urx < ink.Urx || box.Ury < ink.Ury || box.Llx > ink.Llx || box.Lly > ink.Lly {
		t.Fatalf("highlight appearance %+v outside page %+v", ink, box)
	}
}

func TestExpandPageRespectsInheritedCropAndSharedBoxes(t *testing.T) {
	parent := core.MakeDict()
	parent.Set("CropBox", core.MakeArrayFromFloats([]float64{50, 50, 550, 750}))
	parentObject := core.MakeIndirectObject(parent)
	media := &pdf.PdfRectangle{Urx: 600, Ury: 800}
	page := pdf.NewPdfPage()
	page.MediaBox, page.Parent = media, parentObject
	bounds := annotationBounds{}
	bounds.include(pdf.PdfRectangle{Llx: -25, Lly: -30, Urx: 650, Ury: 850})
	if err := bounds.expandPage(page); err != nil {
		t.Fatal(err)
	}
	if *page.MediaBox != *bounds.rect || page.CropBox == nil || *page.CropBox != *bounds.rect {
		t.Fatalf("inherited clipping remains: media=%+v crop=%+v", page.MediaBox, page.CropBox)
	}
	if *media != (pdf.PdfRectangle{Urx: 600, Ury: 800}) {
		t.Fatal("shared media box mutated")
	}
	arr, _ := core.GetArray(parent.Get("CropBox"))
	crop, err := pdf.NewPdfRectangle(*arr)
	if err != nil || *crop != (pdf.PdfRectangle{Llx: 50, Lly: 50, Urx: 550, Ury: 750}) {
		t.Fatal("inherited crop box mutated", err)
	}
}

func TestExportIgnoresEraserAndEmptyStrokesForPageBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		points []rm.Point
		brush  rm.BrushType
	}{
		{"eraser", []rm.Point{{X: -100, Y: -100}, {X: 1700, Y: 2100}}, rm.Eraser},
		{"empty", nil, rm.FinelinerV5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := generateBoundsFixture(t, tc.points, tc.brush, nil)
			p := boundsPage(t, r, 1)
			if *p.MediaBox != (pdf.PdfRectangle{Urx: 600, Ury: 800}) {
				t.Fatalf("non-rendered ink changed page dimensions: %+v", p.MediaBox)
			}
		})
	}
}

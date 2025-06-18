package archive

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWrite(t *testing.T) {
	zip := NewZip()
	zip.Content.FileType = "pdf"
	zip.Content.PageCount = 1
	zip.Pages = append(zip.Pages, Page{Pagedata: "Blank"})
	zip.Payload = []byte{'p', 'd', 'f'}

	// create test file
	file, err := os.Create("write.zip")
	if err != nil {
		t.Error(err)
	}
	defer file.Close()

	// read file into note
	err = zip.Write(file)
	if err != nil {
		t.Error(err)
	}
}

func TestCoverPageNumber(t *testing.T) {
	// Test with coverpage set to first page (0)
	val := 0
	cstr, err := createZipContent("pdf", []string{""}, &val)
	if err != nil {
		t.Fatal(err)
	}

	var c Content
	if err := json.Unmarshal([]byte(cstr), &c); err != nil {
		t.Fatal(err)
	}

	if c.CoverPageNumber == nil || *c.CoverPageNumber != 0 {
		t.Fatalf("expected coverPageNumber 0 got %v", c.CoverPageNumber)
	}

	// Test with no coverpage set
	cstr, err = createZipContent("pdf", []string{""}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var c2 Content
	if err := json.Unmarshal([]byte(cstr), &c2); err != nil {
		t.Fatal(err)
	}

	if c2.CoverPageNumber != nil {
		t.Fatalf("expected no coverPageNumber got %v", c2.CoverPageNumber)
	}
}

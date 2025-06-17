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
	os.Setenv("RMAPI_COVERPAGE", "first")
	defer os.Unsetenv("RMAPI_COVERPAGE")

	cstr, err := createZipContent("pdf", []string{""})
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
}

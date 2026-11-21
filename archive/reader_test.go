package archive

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	zip := NewZip()

	// open test file
	file, err := os.Open("test.zip")
	if err != nil {
		t.Error(err)
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		t.Error(err)
	}

	// read file into note
	err = zip.Read(file, fi.Size())
	if err != nil {
		t.Error(err)
	}
}

func TestReadPagedataIsOptional(t *testing.T) {
	fixture, err := zip.OpenReader("test.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()

	for _, tc := range []struct {
		name     string
		pagedata []string
	}{
		{name: "missing"},
		{name: "present", pagedata: []string{"Lined"}},
		{name: "duplicate", pagedata: []string{"Lined", "Blank"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var data bytes.Buffer
			writer := zip.NewWriter(&data)
			for _, file := range fixture.File {
				if strings.HasSuffix(file.Name, ".pagedata") {
					continue
				}
				if err := writer.Copy(file); err != nil {
					t.Fatal(err)
				}
			}
			for _, template := range tc.pagedata {
				file, err := writer.Create(template + ".pagedata")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := file.Write([]byte(template + "\n")); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			archive := NewZip()
			err := archive.Read(bytes.NewReader(data.Bytes()), int64(data.Len()))
			if len(tc.pagedata) > 1 {
				if err == nil || !strings.Contains(err.Error(), "unique pagedata file") {
					t.Fatalf("expected duplicate pagedata error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(archive.Pages) == 0 || archive.Pages[0].Data == nil {
				t.Fatal("expected the notebook strokes to still be read")
			}
			want := ""
			if len(tc.pagedata) == 1 {
				want = tc.pagedata[0]
			}
			if got := archive.Pages[0].Pagedata; got != want {
				t.Errorf("page template = %q, want %q", got, want)
			}
		})
	}
}

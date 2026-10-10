package archive

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestZipFile(t *testing.T) {
	d, err := CreateZipDocument("1234", "zipdoc_test.pdf")
	fmt.Println(d)
	if err != nil {
		t.Error(err)
	}

}

func TestCreateZipContentTags(t *testing.T) {
	raw, err := createZipContent("pdf", nil, nil, nil, nil, nil, []string{"book", "fiction", ""})
	if err != nil {
		t.Fatal(err)
	}

	var content Content
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatal(err)
	}

	if len(content.DocumentTags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %+v", len(content.DocumentTags), content.DocumentTags)
	}
	if content.DocumentTags[0].Name != "book" || content.DocumentTags[1].Name != "fiction" {
		t.Errorf("unexpected tag names: %+v", content.DocumentTags)
	}
	for _, tag := range content.DocumentTags {
		if tag.Timestamp <= 0 {
			t.Errorf("tag %q has no timestamp", tag.Name)
		}
	}
}

func TestCreateZipContentNoTags(t *testing.T) {
	raw, err := createZipContent("pdf", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	var content Content
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		t.Fatal(err)
	}
	if len(content.DocumentTags) != 0 {
		t.Errorf("expected no tags, got %+v", content.DocumentTags)
	}
}

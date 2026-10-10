package archive

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func decodeContent(t *testing.T, raw []byte) (map[string]json.RawMessage, []Tag) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	var tags []Tag
	if err := json.Unmarshal(fields["tags"], &tags); err != nil {
		t.Fatal(err)
	}
	return fields, tags
}

func TestSetContentTagsPreservesUnknownFields(t *testing.T) {
	raw := []byte(`{
		"formatVersion": 2,
		"fileType": "pdf",
		"cPages": {"pages": [{"id": "p1", "idx": {"timestamp": "1:2", "value": "ba"}}]},
		"zoomMode": "bestFit",
		"tags": []
	}`)

	out, err := SetContentTags(raw, []string{"case"})
	if err != nil {
		t.Fatal(err)
	}

	fields, tags := decodeContent(t, out)
	assert.JSONEq(t, `2`, string(fields["formatVersion"]))
	assert.JSONEq(t, `"pdf"`, string(fields["fileType"]))
	assert.JSONEq(t, `{"pages": [{"id": "p1", "idx": {"timestamp": "1:2", "value": "ba"}}]}`, string(fields["cPages"]))
	assert.JSONEq(t, `"bestFit"`, string(fields["zoomMode"]))
	assert.Len(t, tags, 1)
	assert.Equal(t, "case", tags[0].Name)
	assert.NotZero(t, tags[0].Timestamp)
}

func TestSetContentTagsKeepsTimestampOfExistingTag(t *testing.T) {
	raw := []byte(`{"tags": [{"name": "case", "timestamp": 1700000000000}, {"name": "old", "timestamp": 1600000000000}]}`)

	out, err := SetContentTags(raw, []string{"prep sheet", "case"})
	if err != nil {
		t.Fatal(err)
	}

	_, tags := decodeContent(t, out)
	assert.Equal(t, []string{"prep sheet", "case"}, []string{tags[0].Name, tags[1].Name})
	assert.NotZero(t, tags[0].Timestamp)
	assert.Equal(t, int64(1700000000000), tags[1].Timestamp)
}

func TestSetContentTagsDropsEmptyAndDuplicateNames(t *testing.T) {
	out, err := SetContentTags([]byte(`{}`), []string{"", "case", "case", "emba"})
	if err != nil {
		t.Fatal(err)
	}

	_, tags := decodeContent(t, out)
	assert.Equal(t, 2, len(tags))
	assert.Equal(t, "case", tags[0].Name)
	assert.Equal(t, "emba", tags[1].Name)
}

func TestSetContentTagsClearsWithNoNames(t *testing.T) {
	out, err := SetContentTags([]byte(`{"tags": [{"name": "case", "timestamp": 1}]}`), nil)
	if err != nil {
		t.Fatal(err)
	}

	fields, tags := decodeContent(t, out)
	assert.Empty(t, tags)
	assert.JSONEq(t, `[]`, string(fields["tags"]))
}

func TestSetContentTagsRejectsNonObject(t *testing.T) {
	_, err := SetContentTags([]byte(`[1,2]`), []string{"case"})
	assert.Error(t, err)
}

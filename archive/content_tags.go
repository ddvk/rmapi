package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// SetContentTags returns a copy of a document's raw .content JSON with its
// document-level "tags" array replaced by the given names, in order.
//
// The rewrite is done on the raw JSON object rather than through the Content
// struct so that every field the tablet stores but this library does not model
// (formatVersion, zoom settings, CRDT page lists, …) survives untouched. A tag
// that is already present keeps its original timestamp; new tags are stamped
// with the current time in milliseconds, matching what the tablet writes.
// Duplicate and empty names are dropped.
func SetContentTags(raw []byte, names []string) ([]byte, error) {
	fields := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("content is not a JSON object: %w", err)
		}
	}

	existing := map[string]int64{}
	if current, ok := fields["tags"]; ok && len(current) > 0 {
		var tags []Tag
		if err := json.Unmarshal(current, &tags); err != nil {
			return nil, fmt.Errorf("cannot parse existing tags: %w", err)
		}
		for _, t := range tags {
			if _, dup := existing[t.Name]; !dup {
				existing[t.Name] = t.Timestamp
			}
		}
	}

	now := time.Now().UnixNano() / 1000000
	tags := make([]Tag, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		ts, ok := existing[name]
		if !ok || ts == 0 {
			ts = now
		}
		tags = append(tags, Tag{Name: name, Timestamp: ts})
	}

	encoded, err := json.Marshal(tags)
	if err != nil {
		return nil, err
	}
	fields["tags"] = encoded

	return json.MarshalIndent(fields, "", "    ")
}

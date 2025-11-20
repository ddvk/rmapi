package sync15

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
)

type BlobDoc struct {
	Files []*Entry
	Entry
	Metadata archive.MetadataFile
}

func NewBlobDoc(name, documentId, colType, parentId string) *BlobDoc {
	return &BlobDoc{
		Metadata: archive.MetadataFile{
			DocName:        name,
			CollectionType: colType,
			LastModified:   archive.UnixTimestamp(),
			Parent:         parentId,
		},
		Entry: Entry{
			DocumentID: documentId,
		},
	}

}

func (d *BlobDoc) Rehash() error {

	hash, err := HashEntries(d.Files)
	if err != nil {
		return err
	}
	log.Trace.Println("New doc hash: ", hash)
	d.Hash = hash
	return nil
}

func (d *BlobDoc) MetadataHashAndReader() (hash string, reader io.Reader, err error) {
	jsn, err := json.Marshal(d.Metadata)
	if err != nil {
		return
	}
	sha := sha256.New()
	sha.Write(jsn)
	hash = hex.EncodeToString(sha.Sum(nil))
	log.Trace.Println("new hash", hash)
	reader = bytes.NewReader(jsn)
	found := false
	for _, f := range d.Files {
		if strings.HasSuffix(f.DocumentID, ".metadata") {
			f.Hash = hash
			f.Size = int64(len(jsn))
			found = true
			break
		}
	}
	if !found {
		err = errors.New("metadata not found")
	}

	return
}

func (d *BlobDoc) AddFile(e *Entry) error {
	d.Files = append(d.Files, e)
	size := int64(0)
	for _, f := range d.Files {
		size += f.Size
	}
	d.Size = size
	return d.Rehash()
}

func (t *HashTree) Add(d *BlobDoc) error {
	if len(d.Files) == 0 {
		return errors.New("no files")
	}
	t.Docs = append(t.Docs, d)
	return t.Rehash()
}

func (d *BlobDoc) IndexReader() (io.Reader, error) {
	return d.IndexReaderWithSchema("")
}

func (d *BlobDoc) IndexReaderWithSchema(schema string) (io.Reader, error) {
	if len(d.Files) == 0 {
		return nil, errors.New("no files")
	}

	if schema == "" {
		schema = SchemaVersionV3
	}

	var w bytes.Buffer
	w.WriteString(schema)
	w.WriteString("\n")
	for _, f := range d.Files {
		w.WriteString(f.Line())
		w.WriteString("\n")
	}

	return bytes.NewReader(w.Bytes()), nil
}

// ReadMetadata the document metadata from remote blob
func (d *BlobDoc) ReadMetadata(fileEntry *Entry, r RemoteStorage) error {
	if strings.HasSuffix(fileEntry.DocumentID, ".metadata") {
		log.Trace.Println("Reading metadata: " + d.DocumentID)

		metadata := archive.MetadataFile{}

		meta, err := r.GetReader(fileEntry.Hash, fileEntry.DocumentID)
		if err != nil {
			return err
		}
		defer meta.Close()
		content, err := io.ReadAll(meta)
		if err != nil {
			return err
		}
		err = json.Unmarshal(content, &metadata)
		if err != nil {
			log.Error.Printf("cannot read metadata %s %v", fileEntry.DocumentID, err)
		}
		log.Trace.Println("name from metadata: ", metadata.DocName)
		d.Metadata = metadata
	}

	return nil
}

func (d *BlobDoc) Line() string {
	return d.LineWithSchema("")
}

func (d *BlobDoc) LineWithSchema(schema string) string {
	var sb strings.Builder
	if d.Hash == "" {
		log.Error.Print("missing hash for: ", d.DocumentID)
	}
	sb.WriteString(d.Hash)
	sb.WriteRune(Delimiter)

	typeField := FileType
	if schema == SchemaVersionV3 {
		typeField = DocType
	}
	sb.WriteString(typeField)
	sb.WriteRune(Delimiter)
	sb.WriteString(d.DocumentID)
	sb.WriteRune(Delimiter)

	numFilesStr := strconv.Itoa(len(d.Files))
	sb.WriteString(numFilesStr)
	sb.WriteRune(Delimiter)
	sb.WriteString(strconv.FormatInt(d.Size, 10))
	return sb.String()
}

// Mirror updates the document to be the same as the remote
func (d *BlobDoc) Mirror(e *Entry, r RemoteStorage) error {
	d.Entry = *e
	entryIndex, err := r.GetReader(e.Hash, e.DocumentID)
	if err != nil {
		return err
	}
	defer entryIndex.Close()
	entries, _, err := parseIndex(entryIndex)
	if err != nil {
		return fmt.Errorf("blobdoc index error %v", err)
	}

	head := make([]*Entry, 0)
	current := make(map[string]*Entry)
	new := make(map[string]*Entry)

	for _, e := range entries {
		new[e.DocumentID] = e
	}

	//updated and existing
	for _, currentEntry := range d.Files {
		if newEntry, ok := new[currentEntry.DocumentID]; ok {
			if newEntry.Hash != currentEntry.Hash {
				err = d.ReadMetadata(newEntry, r)
				if err != nil {
					return err
				}
				currentEntry.Hash = newEntry.Hash
			}
			head = append(head, currentEntry)
			current[currentEntry.DocumentID] = currentEntry
		}
	}

	//add missing
	for k, newEntry := range new {
		if _, ok := current[k]; !ok {
			err = d.ReadMetadata(newEntry, r)
			if err != nil {
				return err
			}
			head = append(head, newEntry)
		}
	}
	sort.Slice(head, func(i, j int) bool { return head[i].DocumentID < head[j].DocumentID })
	d.Files = head
	return nil

}
func (d *BlobDoc) ToDocument() *model.Document {
	var lastModified string
	unixTime, err := strconv.ParseInt(d.Metadata.LastModified, 10, 64)
	if err == nil {
		//HACK: convert wrong nano timestamps to millis
		if len(d.Metadata.LastModified) > 18 {
			unixTime /= 1000000
		}

		t := time.Unix(unixTime/1000, 0)
		lastModified = t.UTC().Format(time.RFC3339Nano)
	}
	return &model.Document{
		ID:             d.DocumentID,
		Name:           d.Metadata.DocName,
		Version:        d.Metadata.Version,
		Parent:         d.Metadata.Parent,
		Type:           d.Metadata.CollectionType,
		CurrentPage:    d.Metadata.LastOpenedPage,
		ModifiedClient: lastModified,
	}
}

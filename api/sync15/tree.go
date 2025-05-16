package sync15

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/transport"
	"golang.org/x/sync/errgroup"
)

const SchemaVersionV3 = "3"
const SchemaVersionV4 = "4"

const DocType = "80000000"
const FileType = "0"
const Delimiter = ':'

func FileHashAndSize(file string) ([]byte, int64, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	hasher := sha256.New()
	io.Copy(hasher, f)
	h := hasher.Sum(nil)
	size, err := f.Seek(0, io.SeekCurrent)
	return h, size, err

}

func parseEntry(line string) (*Entry, error) {
	entry := Entry{}
	rdr := NewFieldReader(line)
	numFields := len(rdr.fields)
	if numFields != 5 {
		return nil, fmt.Errorf("wrong number of fields %d", numFields)

	}
	var err error
	entry.Hash, err = rdr.Next()
	if err != nil {
		return nil, err
	}
	entry.Type, err = rdr.Next()
	if err != nil {
		return nil, err
	}
	entry.DocumentID, err = rdr.Next()
	if err != nil {
		return nil, err
	}
	tmp, err := rdr.Next()
	if err != nil {
		return nil, err
	}
	entry.Subfiles, err = strconv.Atoi(tmp)
	if err != nil {
		return nil, fmt.Errorf("cannot read subfiles %s %v", line, err)
	}
	tmp, err = rdr.Next()
	if err != nil {
		return nil, err
	}
	entry.Size, err = strconv.ParseInt(tmp, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("cannot read size %s %v", line, err)
	}
	return &entry, nil
}

func parseSchemaV4(line string) (entriesCount int, totalSize int64, err error) {
	r := NewFieldReader(line)
	_, _ = r.Next()                //0
	_, _ = r.Next()                //.
	entriesCountStr, _ := r.Next() //count?
	totalSizeStr, _ := r.Next()    //size?

	entriesCount, err = strconv.Atoi(entriesCountStr)
	if err != nil {
		return
	}
	totalSize, err = strconv.ParseInt(totalSizeStr, 10, 64)
	if err != nil {
		return
	}

	return
}

func parseIndex(f io.Reader) ([]*Entry, error) {
	var entries []*Entry
	scanner := bufio.NewScanner(f)
	eof := scanner.Scan()
	if !eof {
		return nil, fmt.Errorf("empty index file")
	}
	schema := scanner.Text()
	expectedCount := 0
	count := 0
	var err error
	switch schema {

	case SchemaVersionV4:
		eof := scanner.Scan()
		if !eof {
			return nil, fmt.Errorf("expecting a schema v4 line")
		}
		line := scanner.Text()
		expectedCount, _, err = parseSchemaV4(line)
		if err != nil {
			return nil, fmt.Errorf("can't parse v4 line %v", err)
		}
		fallthrough
	case SchemaVersionV3:
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				log.Warning.Printf("TODO: empty line in index file, ignored")
				continue
			}
			count++
			entry, err := parseEntry(line)
			if err != nil {
				return nil, fmt.Errorf("cant parse line '%s', %w", line, err)
			}

			entries = append(entries, entry)
		}
	default:
		return nil, fmt.Errorf("unsupported schema %s", schema)
	}
	if schema == SchemaVersionV4 {
		if count != expectedCount {
			log.Warning.Printf("entries mismatch, expected %d, but was %d", expectedCount, count)
		}
	}

	return entries, nil
}

func (t *HashTree) IndexReader() (io.Reader, error) {
	var w bytes.Buffer
	w.WriteString(SchemaVersionV3)
	w.WriteString("\n")
	for _, d := range t.Docs {
		w.WriteString(d.Line())
		w.WriteString("\n")
	}

	return bytes.NewReader(w.Bytes()), nil
}

type HashTree struct {
	Hash         string
	Generation   int64
	Docs         []*BlobDoc
	CacheVersion int
}

func (t *HashTree) FindDoc(id string) (*BlobDoc, error) {
	//O(n)
	for _, d := range t.Docs {
		if d.DocumentID == id {
			return d, nil
		}
	}
	return nil, fmt.Errorf("doc %s not found", id)
}

func (t *HashTree) Remove(id string) error {
	docIndex := -1
	for index, d := range t.Docs {
		if d.DocumentID == id {
			docIndex = index
			break
		}
	}
	if docIndex > -1 {
		log.Trace.Printf("Removing %s", id)
		length := len(t.Docs) - 1
		t.Docs[docIndex] = t.Docs[length]
		t.Docs = t.Docs[:length]

		t.Rehash()
		return nil
	}
	return fmt.Errorf("%s not found", id)
}

func (t *HashTree) Rehash() error {
	entries := []*Entry{}
	for _, e := range t.Docs {
		entries = append(entries, &e.Entry)
	}
	hash, err := HashEntries(entries)
	if err != nil {
		return err
	}
	log.Info.Println("New root hash: ", hash)
	t.Hash = hash
	return nil
}

// adds the extensions to filename (for the rm-file header later)
func addExt(name string, ext archive.RmExt) string {
	return name + "." + string(ext)
}

// / Mirror makes the tree look like the storage
func (t *HashTree) Mirror(r RemoteStorage, maxconcurrent int) error {
	rootHash, gen, err := r.GetRootIndex()
	if err != nil && err != transport.ErrNotFound {
		return err
	}
	if rootHash == "" && gen == 0 {
		log.Info.Println("Empty cloud")
		t.Docs = nil
		t.Generation = 0
		return nil
	}

	if rootHash == t.Hash {
		return nil
	}
	log.Info.Printf("remote root hash different")

	rootIndexReader, err := r.GetReader(rootHash, addExt("root", archive.DocSchemaExt))
	if err != nil {
		return fmt.Errorf("cannot get root hash %v", err)
	}
	defer rootIndexReader.Close()

	entries, err := parseIndex(rootIndexReader)
	if err != nil {
		return fmt.Errorf("cannot parse rootIndex, %v", err)
	}

	head := make([]*BlobDoc, 0)
	current := make(map[string]*BlobDoc)
	new := make(map[string]*Entry)

	for _, e := range entries {
		new[e.DocumentID] = e
	}
	wg, ctx := errgroup.WithContext(context.TODO())
	wg.SetLimit(maxconcurrent)

	//current documents
	for _, doc := range t.Docs {
		if entry, ok := new[doc.DocumentID]; ok {
			head = append(head, doc)
			current[doc.DocumentID] = doc

			if entry.Hash != doc.Hash {
				log.Info.Println("doc updated: ", doc.DocumentID)
				e := entry
				d := doc
				wg.Go(func() error {
					return d.Mirror(e, r)
				})
			}
		}
		select {
		case <-ctx.Done():
			goto EXIT
		default:
		}
	}

	//find new entries
	for k, newEntry := range new {
		if _, ok := current[k]; !ok {
			doc := &BlobDoc{}
			log.Trace.Println("doc new: ", k)
			head = append(head, doc)
			e := newEntry
			wg.Go(func() error {
				return doc.Mirror(e, r)
			})
		}
		select {
		case <-ctx.Done():
			goto EXIT
		default:
		}
	}
EXIT:
	err = wg.Wait()
	if err != nil {
		return fmt.Errorf("was not ok: %v", err)
	}
	sort.Slice(head, func(i, j int) bool { return head[i].DocumentID < head[j].DocumentID })
	t.Docs = head
	t.Generation = gen
	t.Hash = rootHash
	return nil
}

func BuildTree(provider RemoteStorage) (*HashTree, error) {
	tree := HashTree{}

	rootHash, gen, err := provider.GetRootIndex()

	if err != nil {
		return nil, err
	}
	tree.Hash = rootHash
	tree.Generation = gen

	rootIndex, err := provider.GetReader(rootHash, "roothash")
	if err != nil {
		return nil, err
	}

	defer rootIndex.Close()
	entries, err := parseIndex(rootIndex)
	if err != nil {
		return nil, fmt.Errorf("build tree root index error %v", err)
	}

	for _, e := range entries {
		f, _ := provider.GetReader(e.Hash, e.DocumentID)
		defer f.Close()

		doc := &BlobDoc{}
		doc.Entry = *e

		items, _ := parseIndex(f)
		doc.Files = items
		for _, i := range items {
			doc.ReadMetadata(i, provider)
		}
		//don't include deleted items
		if doc.Metadata.Deleted {
			continue
		}

		tree.Docs = append(tree.Docs, doc)
	}

	return &tree, nil

}

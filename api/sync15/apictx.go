package sync15

import (
	"archive/zip"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/config"
	"github.com/juruen/rmapi/filetree"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/transport"
	"github.com/juruen/rmapi/util"
)

// An ApiCtx allows you interact with the remote reMarkable API
type ApiCtx struct {
	Http        *transport.HttpClientCtx
	ft          *filetree.FileTreeCtx
	blobStorage *BlobStorage
	hashTree    *HashTree
}

// max number of concurrent requests
var concurrent = 20

func init() {
	c := os.Getenv("RMAPI_CONCURRENT")
	if u, err := strconv.Atoi(c); err == nil {
		concurrent = u
	}
}

func CreateCtx(http *transport.HttpClientCtx) (*ApiCtx, error) {
	apiStorage := NewBlobStorage(http)
	cacheTree, err := loadTree()
	if err != nil {
		fmt.Print(err)
		return nil, err
	}
	err = cacheTree.Mirror(apiStorage, concurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to mirror %v", err)
	}
	saveTree(cacheTree)
	// Note: We don't save diff snapshot here to avoid resetting it during token refresh.
	// Snapshot is only saved on explicit Refresh() calls.
	tree := DocumentsFileTree(cacheTree)
	return &ApiCtx{http, tree, apiStorage, cacheTree}, nil
}

func (ctx *ApiCtx) Filetree() *filetree.FileTreeCtx {
	return ctx.ft
}

func (ctx *ApiCtx) HashTree() *HashTree {
	return ctx.hashTree
}

func (ctx *ApiCtx) Refresh() (string, int64, error) {
	err := ctx.hashTree.Mirror(ctx.blobStorage, concurrent)
	if err != nil {
		return "", 0, err
	}
	ctx.ft = DocumentsFileTree(ctx.hashTree)
	// Save snapshot after refresh for diff comparison
	saveDiffSnapshot(ctx.hashTree)
	return ctx.hashTree.Hash, ctx.hashTree.Generation, nil
}

// RefreshTree syncs the file tree with remote but does not save snapshot
func (ctx *ApiCtx) RefreshTree() (string, int64, error) {
	err := ctx.hashTree.Mirror(ctx.blobStorage, concurrent)
	if err != nil {
		return "", 0, err
	}
	ctx.ft = DocumentsFileTree(ctx.hashTree)
	// Note: We don't save snapshot here - that's only done in Refresh()
	return ctx.hashTree.Hash, ctx.hashTree.Generation, nil
}

// RefreshToken updates the user token in the existing context without recreating it
func (ctx *ApiCtx) RefreshToken() error {
	// Get new user token using existing device token
	// Use transport directly to avoid import cycle
	resp := transport.BodyString{}
	err := ctx.Http.Post(transport.DeviceBearer, config.NewUserDevice, nil, &resp)
	if err != nil {
		return fmt.Errorf("failed to refresh user token: %v", err)
	}
	
	userToken := resp.Content
	
	// Update token in the context
	ctx.Http.Tokens.UserToken = userToken
	
	// Save to config
	configPath, err := config.ConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %v", err)
	}
	authTokens := config.LoadTokens(configPath)
	authTokens.UserToken = userToken
	config.SaveTokens(configPath, authTokens)
	
	return nil
}

// Diff refreshes the file tree and compares it to the last saved snapshot
// After comparison, it updates the snapshot so subsequent calls will show no changes if nothing changed
func (ctx *ApiCtx) Diff() (*DiffResult, error) {
	// First refresh the tree to get latest state
	_, _, err := ctx.RefreshTree()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh tree: %v", err)
	}
	
	// Then compare with snapshot
	snapshot, err := loadDiffSnapshot()
	if err != nil {
		return nil, fmt.Errorf("failed to load diff snapshot: %v", err)
	}
	
	result := computeDiff(ctx.hashTree, snapshot)
	
	// Update snapshot after comparison so next diff call will compare against this state
	err = saveDiffSnapshot(ctx.hashTree)
	if err != nil {
		// Log error but don't fail - the diff result is still valid
		log.Warning.Printf("Failed to save diff snapshot: %v", err)
	}
	
	return result, nil
}

// Nuke removes all documents from the account
func (ctx *ApiCtx) Nuke() (err error) {
	err = Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		ctx.hashTree.Docs = nil
		ctx.hashTree.Rehash()
		return nil
	}, true)
	return err
}

// FetchDocument downloads a document given its ID and saves it locally into dstPath
func (ctx *ApiCtx) FetchDocument(docId, dstPath string) error {
	doc, err := ctx.hashTree.FindDoc(docId)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "rmapizip")

	if err != nil {
		log.Error.Println("failed to create tmpfile for zip dir", err)
		return err
	}
	defer tmp.Close()

	w := zip.NewWriter(tmp)
	defer w.Close()
	for _, f := range doc.Files {
		log.Trace.Println("fetching document: ", f.DocumentID)
		blobReader, err := ctx.blobStorage.GetReader(f.Hash, f.DocumentID)
		if err != nil {
			return err
		}
		defer blobReader.Close()
		header := zip.FileHeader{}
		header.Name = f.DocumentID
		header.Modified = time.Now()
		zipWriter, err := w.CreateHeader(&header)
		if err != nil {
			return err
		}
		_, err = io.Copy(zipWriter, blobReader)

		if err != nil {
			return err
		}
	}
	w.Close()
	tmpPath := tmp.Name()
	_, err = util.CopyFile(tmpPath, dstPath)

	if err != nil {
		log.Error.Printf("failed to copy %s to %s, er: %s\n", tmpPath, dstPath, err.Error())
		return err
	}

	defer os.RemoveAll(tmp.Name())

	return nil
}

// CreateDir creates a remote directory with a given name under the parentId directory
func (ctx *ApiCtx) CreateDir(parentId, name string, notify bool) (*model.Document, error) {
	var err error

	files := &archive.DocumentFiles{}

	tmpDir, err := os.MkdirTemp("", "rmupload")
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	objectName, filePath, err := archive.CreateMetadata(id, name, parentId, model.DirectoryType, tmpDir)
	if err != nil {
		return nil, err
	}
	files.AddMap(objectName, filePath, archive.MetadataExt)

	objectName, filePath, err = archive.CreateContent(id, "", tmpDir, nil, nil)
	if err != nil {
		return nil, err
	}
	files.AddMap(objectName, filePath, archive.ContentExt)

	doc := NewBlobDoc(name, id, model.DirectoryType, parentId)

	for _, f := range files.Files {
		log.Info.Printf("File %s, path: %s", f.Name, f.Path)
		hash, size, err := FileHashAndSize(f.Path)
		if err != nil {
			return nil, err
		}
		hashStr := hex.EncodeToString(hash)
		fileEntry := &Entry{
			DocumentID: f.Name,
			Hash:       hashStr,
			Type:       FileType,
			Size:       size,
		}
		reader, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		//does not accept rm-file in header
		err = ctx.blobStorage.UploadBlob(hashStr, f.Name, reader)
		reader.Close()

		if err != nil {
			return nil, err
		}

		doc.AddFile(fileEntry)
	}

	log.Info.Println("Uploading new doc index...", doc.Hash)
	indexReader, err := doc.IndexReader()
	if err != nil {
		return nil, err
	}
	// defer indexReader.Close()
	err = ctx.blobStorage.UploadBlob(doc.Hash, addExt(doc.DocumentID, archive.DocSchemaExt), indexReader)
	if err != nil {
		return nil, err
	}

	err = Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		return t.Add(doc)
	}, notify)

	if err != nil {
		return nil, err
	}

	if notify {
		err = ctx.SyncComplete()
		if err != nil {
			return nil, err
		}
	}

	return doc.ToDocument(), nil
}

// Sync applies changes to the local tree and syncs with the remote storage
func Sync(b *BlobStorage, tree *HashTree, operation func(t *HashTree) error, notify bool) error {
	syncTry := 0
	for {
		syncTry++
		if syncTry > 10 {
			log.Error.Println("Something is wrong")
			break
		}
		log.Info.Println("Syncing...")
		err := operation(tree)
		if err != nil {
			return err
		}

		indexReader, err := tree.IndexReader()
		if err != nil {
			return err
		}
		err = b.UploadBlob(tree.Hash, addExt("root", archive.DocSchemaExt), indexReader)
		if err != nil {
			return err
		}
		// TODO
		// defer indexReader.Close()

		log.Info.Println("updating root, old gen: ", tree.Generation)

		newGeneration, err := b.WriteRootIndex(tree.Hash, tree.Generation, notify)

		if err == nil {
			log.Info.Println("wrote root, new gen: ", newGeneration)
			tree.Generation = newGeneration
			break
		}

		if err != transport.ErrWrongGeneration {
			return err
		}

		log.Info.Println("wrong generation, re-reading remote tree")
		//resync and try again
		err = tree.Mirror(b, concurrent)
		if err != nil {
			return err
		}
		log.Warning.Println("remote tree has changed, refresh the file tree")
	}
	return saveTree(tree)
}

// DeleteEntry removes an entry: either an empty directory or a file
func (ctx *ApiCtx) DeleteEntry(node *model.Node, recursive, notify bool) error {
	if node.IsDirectory() && len(node.Children) > 0 && !recursive {
		return errors.New("directory is not empty")
	}

	err := Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		return t.Remove(node.Document.ID)
	}, notify)
	return err
}

// MoveEntry moves an entry (either a directory or a file)
// - src is the source node to be moved
// - dstDir is an existing destination directory
// - name is the new name of the moved entry in the destination directory
func (ctx *ApiCtx) MoveEntry(src, dstDir *model.Node, name string) (*model.Node, error) {
	if dstDir.IsFile() {
		return nil, errors.New("destination directory is a file")
	}
	var err error

	err = Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		doc, err := t.FindDoc(src.Document.ID)
		if err != nil {
			return err
		}
		doc.Metadata.Version++
		doc.Metadata.DocName = name
		doc.Metadata.Parent = dstDir.Id()
		doc.Metadata.MetadataModified = true

		hashStr, reader, err := doc.MetadataHashAndReader()
		if err != nil {
			return err
		}
		err = doc.Rehash()
		if err != nil {
			return err
		}
		err = t.Rehash()

		if err != nil {
			return err
		}

		err = ctx.blobStorage.UploadBlob(hashStr, addExt(doc.DocumentID, archive.MetadataExt), reader)

		if err != nil {
			return err
		}

		log.Info.Println("Uploading new doc index...", doc.Hash)
		indexReader, err := doc.IndexReader()
		if err != nil {
			return err
		}
		// defer indexReader.Close()
		return ctx.blobStorage.UploadBlob(doc.Hash, addExt(doc.DocumentID, archive.DocSchemaExt), indexReader)
	}, true)

	if err != nil {
		return nil, err
	}

	d, err := ctx.hashTree.FindDoc(src.Document.ID)
	if err != nil {
		return nil, err
	}

	return &model.Node{Document: d.ToDocument(), Children: src.Children, Parent: dstDir}, nil
}

// UploadDocument uploads a local document given by sourceDocPath under the parentId directory
func (ctx *ApiCtx) UploadDocument(parentId string, sourceDocPath string, notify bool, coverpage *int) (*model.Document, error) {
	//TODO: overwrite file
	name, ext := util.DocPathToName(sourceDocPath)

	if name == "" {
		return nil, errors.New("file name is invalid")
	}

	if !util.IsFileTypeSupported(ext) {
		return nil, errors.New("unsupported file extension: " + ext)
	}

	var err error

	tmpDir, err := os.MkdirTemp("", "rmupload")
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(tmpDir)

	docFiles, id, err := archive.Prepare(name, parentId, sourceDocPath, ext, tmpDir, coverpage)
	if err != nil {
		return nil, err
	}

	doc := NewBlobDoc(name, id, model.DocumentType, parentId)
	for _, f := range docFiles.Files {
		log.Info.Printf("File %s, path: %s", f.Name, f.Path)
		hash, size, err := FileHashAndSize(f.Path)
		if err != nil {
			return nil, err
		}
		hashStr := hex.EncodeToString(hash)
		fileEntry := &Entry{
			DocumentID: f.Name,
			Hash:       hashStr,
			Type:       FileType,
			Size:       size,
		}
		reader, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		err = ctx.blobStorage.UploadBlob(hashStr, fileEntry.DocumentID, reader)

		if err != nil {
			return nil, err
		}

		doc.AddFile(fileEntry)
	}

	log.Info.Printf("Uploading new doc index...%s, size: %d", doc.Hash, doc.Size)
	indexReader, err := doc.IndexReader()
	if err != nil {
		return nil, err
	}
	// defer indexReader.Close()
	err = ctx.blobStorage.UploadBlob(doc.Hash, addExt(doc.DocumentID, archive.DocSchemaExt), indexReader)
	if err != nil {
		return nil, err
	}

	err = Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		return t.Add(doc)
	}, notify)

	if err != nil {
		return nil, err
	}

	return doc.ToDocument(), nil
}

// ReplaceDocumentFile replaces the main document file (e.g. PDF) of an existing document
// identified by docId with the local file given by sourceDocPath. Metadata and annotations
// remain untouched.
func (ctx *ApiCtx) ReplaceDocumentFile(docId, sourceDocPath string, notify bool) error {
	_, ext := util.DocPathToName(sourceDocPath)
	return Sync(ctx.blobStorage, ctx.hashTree, func(t *HashTree) error {
		doc, err := t.FindDoc(docId)
		if err != nil {
			return err
		}

		var fileEntry *Entry
		for _, f := range doc.Files {
			if strings.HasSuffix(f.DocumentID, "."+ext) {
				fileEntry = f
				break
			}
		}
		if fileEntry == nil {
			return fmt.Errorf("document does not contain .%s", ext)
		}

		hash, size, err := FileHashAndSize(sourceDocPath)
		if err != nil {
			return err
		}
		hashStr := hex.EncodeToString(hash)

		r, err := os.Open(sourceDocPath)
		if err != nil {
			return err
		}
		defer r.Close()

		if err := ctx.blobStorage.UploadBlob(hashStr, fileEntry.DocumentID, r); err != nil {
			return err
		}

		fileEntry.Hash = hashStr
		fileEntry.Size = size

		if err := doc.Rehash(); err != nil {
			return err
		}
		if err := t.Rehash(); err != nil {
			return err
		}

		indexReader, err := doc.IndexReader()
		if err != nil {
			return err
		}
		return ctx.blobStorage.UploadBlob(doc.Hash, addExt(doc.DocumentID, archive.DocSchemaExt), indexReader)
	}, notify)
}

// DocumentsFileTree reads your remote documents and builds a file tree
// structure to represent them
func DocumentsFileTree(tree *HashTree) *filetree.FileTreeCtx {

	documents := make([]*model.Document, 0)
	for _, d := range tree.Docs {
		//dont show deleted (already cached)
		if d.Metadata.Deleted {
			continue
		}
		doc := d.ToDocument()
		documents = append(documents, doc)
	}

	fileTree := filetree.CreateFileTreeCtx()

	for _, d := range documents {
		log.Trace.Printf("adding: %s docid: %s ", d.Name, d.ID)
		fileTree.AddDocument(d)
	}
	fileTree.FinishAdd()

	return &fileTree
}

// SyncComplete notfies that somethings has changed (triggers tablet sync)
func (ctx *ApiCtx) SyncComplete() error {
	err := ctx.blobStorage.SyncComplete(ctx.hashTree.Generation)

	//sync can be called once per generation, ignore the error if nothing was changed
	if err == transport.ErrConflict {
		log.Trace.Printf("ignoring error: %v", err)
		return nil
	}

	if err != nil {
		log.Error.Printf("cannot send sync %v", err)
	}
	return nil
}

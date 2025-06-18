package archive

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/juruen/rmapi/log"
	"github.com/juruen/rmapi/model"
	"github.com/juruen/rmapi/util"
)

// extensions of the internal rm files
type RmExt string

const (
	// DocSchema
	DocSchemaExt RmExt = "docSchema"
	MetadataExt  RmExt = "metadata"
	ContentExt   RmExt = "content"
)

type NamePath struct {
	Name     string
	Path     string
	FileType RmExt
}

type DocumentFiles struct {
	Files []NamePath
}

func (d *DocumentFiles) AddMap(name, filepath string, filetype RmExt) {
	fs := NamePath{
		Name:     name,
		Path:     filepath,
		FileType: filetype,
	}
	d.Files = append(d.Files, fs)
}

// Prepare prepares a file for uploading (creates needed temp files or unpacks a zip)
func Prepare(name, parentId, sourceDocPath, ext, tmpDir string, coverpage *int) (files *DocumentFiles, id string, err error) {
	files = &DocumentFiles{}
	if ext == util.ZIP || ext == util.RMDOC {
		var metadataPath string
		id, files, metadataPath, err = Unpack(sourceDocPath, tmpDir)
		if err != nil {
			return
		}
		if id == "" {
			return nil, "", errors.New("could not determine the Document UUID")
		}
		if metadataPath == "" {
			log.Warning.Println("missing metadata, creating...", name)
			objectName, filePath, err1 := CreateMetadata(id, name, parentId, model.DocumentType, tmpDir)
			if err1 != nil {
				err = err1
				return
			}
			files.AddMap(objectName, filePath, MetadataExt)
		} else {
			err = FixMetadata(parentId, name, metadataPath)
			if err != nil {
				return
			}
		}
	} else {
		id = uuid.New().String()
		objectName := id + "." + ext
		doctype := ext
		var pageIds []string
		if ext == util.RM {
			pageId := uuid.New().String()
			objectName = fmt.Sprintf("%s/%s.rm", id, pageId)
			doctype = "notebook"
			pageIds = []string{pageId}
		}
		files.AddMap(objectName, sourceDocPath, RmExt(doctype))
		objectName, filePath, err1 := CreateMetadata(id, name, parentId, model.DocumentType, tmpDir)
		if err1 != nil {
			err = err1
			return
		}
		files.AddMap(objectName, filePath, MetadataExt)

		objectName, filePath, err = CreateContent(id, doctype, tmpDir, pageIds, coverpage)
		if err != nil {
			return
		}
		files.AddMap(objectName, filePath, ContentExt)
	}
	return files, id, err
}

// FixMetadata fixes the metadata with the new parent and filename
func FixMetadata(parentId, name, path string) error {
	meta := MetadataFile{}
	metaData, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(metaData, &meta)
	if err != nil {
		return err
	}
	meta.Parent = parentId
	meta.DocName = name
	meta.LastModified = UnixTimestamp()

	metaData, err = json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, metaData, 0600)
}

// Unpack unpacks a rmapi .zip file
func Unpack(src, dest string) (id string, files *DocumentFiles, metadataPath string, err error) {
	log.Info.Println("Unpacking in: ", dest)
	r, err := zip.OpenReader(src)
	if err != nil {
		return
	}
	defer r.Close()
	files = &DocumentFiles{}

	for _, f := range r.File {
		fname := f.Name

		ext := filepath.Ext(fname)
		if len(ext) > 0 {
			ext = ext[1:]
		}
		if ext == string(ContentExt) {
			id = strings.TrimSuffix(fname, path.Ext(fname))
		}
		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			err = fmt.Errorf("%s: illegal file path", fpath)
			return
		}

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		} else {
			files.AddMap(f.Name, fpath, RmExt(ext))
		}

		if ext == string(MetadataExt) {
			metadataPath = fpath
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return
		}

		outFile, err1 := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err1 != nil {
			err = err1
			return
		}

		rc, err1 := f.Open()
		if err != nil {
			err = err1
			return
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return
		}
	}

	return id, files, metadataPath, nil
}

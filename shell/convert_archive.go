package shell

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juruen/rmapi/archive"
	"github.com/juruen/rmapi/encoding/rm"
)

// ContentFile represents the structure of the .content file in newer Remarkable formats
type ContentFile struct {
	CPages struct {
		Pages []struct {
			ID string `json:"id"`
		} `json:"pages"`
		LastOpened struct {
			Value string `json:"value"`
		} `json:"lastOpened"`
	} `json:"cPages"`
}

// LoadArchive tries to load an archive using the standard rmapi reader first,
// and falls back to the new format parser if the standard reader fails.
func LoadArchive(file *os.File, fileSize int64) (*archive.Zip, error) {
	zipArchive := archive.NewZip()

	// Try the standard rmapi Read first
	err := zipArchive.Read(file, fileSize)
	if err == nil {
		// Check if we got pages with data
		if len(zipArchive.Pages) > 0 {
			hasData := false
			for _, page := range zipArchive.Pages {
				if page.Data != nil && len(page.Data.Layers) > 0 {
					hasData = true
					break
				}
			}
			if hasData {
				return zipArchive, nil
			}
		}
	}

	// If standard read failed or found no pages with data, try new format
	// Reset file position
	file.Seek(0, 0)
	return loadArchiveNewFormat(file)
}

// loadArchiveNewFormat loads archives that don't have .pagedata files
// (newer Remarkable format)
func loadArchiveNewFormat(file *os.File) (*archive.Zip, error) {
	zipArchive := archive.NewZip()

	// Get file size
	fi, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("can't stat file: %w", err)
	}

	// Reset file position
	file.Seek(0, 0)

	// Open as standard ZIP archive
	reader, err := zip.NewReader(file, fi.Size())
	if err != nil {
		return nil, fmt.Errorf("can't open as zip: %w", err)
	}

	// Find the .content file to get page list
	var contentFile *zip.File
	var docUUID string
	for _, f := range reader.File {
		if strings.HasSuffix(f.Name, ".content") {
			contentFile = f
			// Extract UUID from filename (format: UUID.content)
			baseName := filepath.Base(f.Name)
			docUUID = strings.TrimSuffix(baseName, ".content")
			break
		}
	}

	if contentFile == nil {
		return nil, errors.New("no .content file found in archive")
	}

	// Read and parse content file
	contentReader, err := contentFile.Open()
	if err != nil {
		return nil, fmt.Errorf("can't open content file: %w", err)
	}
	defer contentReader.Close()

	contentData, err := io.ReadAll(contentReader)
	if err != nil {
		return nil, fmt.Errorf("can't read content file: %w", err)
	}

	var content ContentFile
	err = json.Unmarshal(contentData, &content)
	if err != nil {
		return nil, fmt.Errorf("can't parse content file: %w", err)
	}

	// Set UUID
	zipArchive.UUID = docUUID

	// Set Content metadata if available
	if len(content.CPages.Pages) > 0 {
		// Find last opened page index
		lastOpenedID := content.CPages.LastOpened.Value
		for i, page := range content.CPages.Pages {
			if page.ID == lastOpenedID {
				zipArchive.Content.LastOpenedPage = i
				break
			}
		}
	}

	// Read each page .rm file
	for _, pageInfo := range content.CPages.Pages {
		pageID := pageInfo.ID
		// Pages are stored in subdirectories: UUID/pageID.rm
		pagePath := fmt.Sprintf("%s/%s.rm", docUUID, pageID)

		var pageFile *zip.File
		for _, f := range reader.File {
			if f.Name == pagePath {
				pageFile = f
				break
			}
		}

		if pageFile == nil {
			continue
		}

		// Read page data
		pageReader, err := pageFile.Open()
		if err != nil {
			continue
		}

		pageData, err := io.ReadAll(pageReader)
		pageReader.Close()
		if err != nil {
			continue
		}

		// Parse .rm file
		page := archive.Page{}
		page.Data = rm.New()
		err = page.Data.UnmarshalBinary(pageData)
		if err != nil {
			continue
		}

		zipArchive.Pages = append(zipArchive.Pages, page)
	}

	if len(zipArchive.Pages) == 0 {
		return nil, errors.New("no pages found in archive")
	}

	return zipArchive, nil
}


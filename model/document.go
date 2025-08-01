package model

const (
	DirectoryType = "CollectionType"
	DocumentType  = "DocumentType"
	TemplateType  = "TemplateType"
)

type Document struct {
	ID             string
	Name           string
	Version        int
	ModifiedClient string
	Type           string
	CurrentPage    int
	Parent         string
}

type BlobRootStorageRequest struct {
	Broadcast  bool   `json:"broadcast"`
	Hash       string `json:"hash"`
	Generation int64  `json:"generation"`
}
type BlobRootStorageResponse struct {
	Hash       string `json:"hash"`
	Generation int64  `json:"generation"`
	Schema     int64  `json:"schemaVersion"`
}

// BlobStorageRequest request
type BlobStorageRequest struct {
	Method       string `json:"http_method"`
	Initial      bool   `json:"initial_sync,omitempty"`
	RelativePath string `json:"relative_path"`
	ParentPath   string `json:"parent_path,omitempty"`
}

// BlobStorageResponse response
type BlobStorageResponse struct {
	Expires            string `json:"expires"`
	Method             string `json:"method"`
	RelativePath       string `json:"relative_path"`
	Url                string `json:"url"`
	MaxUploadSizeBytes int64  `json:"maxuploadsize_bytes,omitifempty"`
}

// SyncCompleteRequest payload of the sync completion
type SyncCompletedRequest struct {
	Generation int64 `json:"generation"`
}

package sync15

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const TrashID = "trash"

// filesEqual compares two Files arrays, excluding metadata files
// Returns true if all non-metadata files have the same hash and size
func filesEqual(files1, files2 []*Entry) bool {
	// Create maps of non-metadata files by DocumentID
	map1 := make(map[string]*Entry)
	map2 := make(map[string]*Entry)

	for _, f := range files1 {
		if !strings.HasSuffix(f.DocumentID, ".metadata") {
			map1[f.DocumentID] = f
		}
	}

	for _, f := range files2 {
		if !strings.HasSuffix(f.DocumentID, ".metadata") {
			map2[f.DocumentID] = f
		}
	}

	// Check if same number of non-metadata files
	if len(map1) != len(map2) {
		return false
	}

	// Check if all files match (same hash and size)
	for id, f1 := range map1 {
		f2, exists := map2[id]
		if !exists {
			return false
		}
		if f1.Hash != f2.Hash || f1.Size != f2.Size {
			return false
		}
	}

	return true
}

// TreeDiffResult contains the differences between two tree states
type TreeDiffResult struct {
	New       []*BlobDoc        `json:"new"`
	Removed   []*BlobDoc        `json:"removed"`
	Modified  []DocDiff         `json:"modified"`
	Moved     []DocDiff         `json:"moved"` // Documents moved without content changes
	NewPaths  map[string]string `json:"newPaths,omitempty"`    // DocumentID -> path for new documents
	RemovedPaths map[string]string `json:"removedPaths,omitempty"` // DocumentID -> path for removed documents
}

// SimplifiedDiffJSON represents a simplified JSON format for diff output
type SimplifiedDiffJSON struct {
	New      []DiffItem   `json:"new,omitempty"`
	Removed  []DiffItem   `json:"removed,omitempty"`
	Moved    []MovedItem  `json:"moved,omitempty"`
	Modified []DiffItem   `json:"modified,omitempty"`
}

// DiffItem represents a document in the simplified diff format
type DiffItem struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
}

// MovedItem represents a moved document in the simplified diff format
type MovedItem struct {
	Path string `json:"path"`
	From string `json:"from"`
	Hash string `json:"hash"`
}

// FormatDiffJSON converts TreeDiffResult to SimplifiedDiffJSON format
func FormatDiffJSON(diff *TreeDiffResult) *SimplifiedDiffJSON {
	result := &SimplifiedDiffJSON{
		New:      []DiffItem{},
		Removed:  []DiffItem{},
		Moved:    []MovedItem{},
		Modified: []DiffItem{},
	}

	// Format new documents
	for _, doc := range diff.New {
		path := diff.NewPaths[doc.DocumentID]
		if path == "" {
			path = doc.Metadata.DocName
		}
		result.New = append(result.New, DiffItem{
			Path: path,
			Hash: doc.Hash,
		})
	}

	// Format removed documents
	for _, doc := range diff.Removed {
		path := diff.RemovedPaths[doc.DocumentID]
		if path == "" {
			path = doc.Metadata.DocName
		}
		result.Removed = append(result.Removed, DiffItem{
			Path: path,
			Hash: doc.Hash,
		})
	}

	// Format moved documents
	for _, moved := range diff.Moved {
		path := moved.Path
		if path == "" {
			path = moved.Name
		}
		from := moved.OldPath
		if from == "" {
			// Try to get from parent change
			if parentChange, ok := moved.Changes["parent"]; ok {
				from = parentChange.Old
			}
		}
		result.Moved = append(result.Moved, MovedItem{
			Path: path,
			From: from,
			Hash: moved.NewHash,
		})
	}

	// Format modified documents
	for _, mod := range diff.Modified {
		path := mod.Path
		if path == "" {
			path = mod.Name
		}
		result.Modified = append(result.Modified, DiffItem{
			Path: path,
			Hash: mod.NewHash,
		})
	}

	return result
}

// DocDiff contains information about a modified document
type DocDiff struct {
	DocumentID string                 `json:"documentId"`
	Name       string                 `json:"name"`
	Path       string                 `json:"path,omitempty"`       // Current path
	OldPath    string                 `json:"oldPath,omitempty"`    // Previous path (for moved docs)
	OldHash    string                 `json:"oldHash"`
	NewHash    string                 `json:"newHash"`
	Changes    map[string]FieldChange `json:"changes"`
}

// FieldChange represents a change in a specific field
type FieldChange struct {
	Old string `json:"old"`
	New string `json:"new"`
}

// DiffTrees compares two HashTree structures and returns the differences
func DiffTrees(current, previous *HashTree) (*TreeDiffResult, error) {
	result := &TreeDiffResult{
		New:         []*BlobDoc{},
		Removed:     []*BlobDoc{},
		Modified:    []DocDiff{},
		Moved:       []DocDiff{},
		NewPaths:    make(map[string]string),
		RemovedPaths: make(map[string]string),
	}

	// Create maps for quick lookup
	currentDocs := make(map[string]*BlobDoc)
	previousDocs := make(map[string]*BlobDoc)

	for _, doc := range current.Docs {
		currentDocs[doc.DocumentID] = doc
	}

	for _, doc := range previous.Docs {
		previousDocs[doc.DocumentID] = doc
	}

	// Find new documents (in current but not in previous)
	// Also include documents that were moved FROM trash (restored)
	for id, doc := range currentDocs {
		if _, exists := previousDocs[id]; !exists {
			result.New = append(result.New, doc)
		} else if previousDoc, exists := previousDocs[id]; exists {
			// Check if document was moved FROM trash (restored)
			if previousDoc.Metadata.Parent == TrashID && doc.Metadata.Parent != TrashID {
				// Document was restored from trash - treat as new
				result.New = append(result.New, doc)
				// Remove from currentDocs so it's not processed as modified
				delete(currentDocs, id)
			}
		}
	}

	// Find removed documents (in previous but not in current)
	// Also include documents moved TO trash
	for id, previousDoc := range previousDocs {
		if currentDoc, exists := currentDocs[id]; !exists {
			result.Removed = append(result.Removed, previousDoc)
		} else {
			// Check if document was moved to trash
			if previousDoc.Metadata.Parent != TrashID && currentDoc.Metadata.Parent == TrashID {
				// Document was moved to trash - treat as removed
				result.Removed = append(result.Removed, previousDoc)
				// Remove from currentDocs so it's not processed as modified
				delete(currentDocs, id)
			}
		}
	}

	// Find modified documents (same ID but different hash or metadata)
	// Skip documents that are in trash (they're already marked as removed)
	for id, currentDoc := range currentDocs {
		if previousDoc, exists := previousDocs[id]; exists {
			// Skip if document is currently in trash (already handled above)
			if currentDoc.Metadata.Parent == TrashID {
				continue
			}
			// Check if hash changed
			if currentDoc.Hash != previousDoc.Hash {
				// Check if actual content files changed (excluding metadata)
				contentChanged := !filesEqual(currentDoc.Files, previousDoc.Files)
				
				changes := make(map[string]FieldChange)
				changes["hash"] = FieldChange{
					Old: previousDoc.Hash,
					New: currentDoc.Hash,
				}

				// Check metadata changes
				if currentDoc.Metadata.DocName != previousDoc.Metadata.DocName {
					changes["name"] = FieldChange{
						Old: previousDoc.Metadata.DocName,
						New: currentDoc.Metadata.DocName,
					}
				}
				parentChanged := currentDoc.Metadata.Parent != previousDoc.Metadata.Parent
				if parentChanged {
					changes["parent"] = FieldChange{
						Old: previousDoc.Metadata.Parent,
						New: currentDoc.Metadata.Parent,
					}
				}
				if currentDoc.Metadata.LastModified != previousDoc.Metadata.LastModified {
					changes["lastModified"] = FieldChange{
						Old: previousDoc.Metadata.LastModified,
						New: currentDoc.Metadata.LastModified,
					}
				}
				if currentDoc.Metadata.Version != previousDoc.Metadata.Version {
					changes["version"] = FieldChange{
						Old: fmt.Sprintf("%d", previousDoc.Metadata.Version),
						New: fmt.Sprintf("%d", currentDoc.Metadata.Version),
					}
				}
				if currentDoc.Metadata.Deleted != previousDoc.Metadata.Deleted {
					changes["deleted"] = FieldChange{
						Old: fmt.Sprintf("%v", previousDoc.Metadata.Deleted),
						New: fmt.Sprintf("%v", currentDoc.Metadata.Deleted),
					}
				}
				if currentDoc.Metadata.Pinned != previousDoc.Metadata.Pinned {
					changes["pinned"] = FieldChange{
						Old: fmt.Sprintf("%v", previousDoc.Metadata.Pinned),
						New: fmt.Sprintf("%v", currentDoc.Metadata.Pinned),
					}
				}
				if currentDoc.Size != previousDoc.Size {
					changes["size"] = FieldChange{
						Old: fmt.Sprintf("%d", previousDoc.Size),
						New: fmt.Sprintf("%d", currentDoc.Size),
					}
				}

				docDiff := DocDiff{
					DocumentID: id,
					Name:       currentDoc.Metadata.DocName,
					OldHash:    previousDoc.Hash,
					NewHash:    currentDoc.Hash,
					Changes:    changes,
				}

				// If only parent changed and content files didn't change, it's a move
				if !contentChanged && parentChanged && len(changes) <= 3 {
					// Only hash, parent, and possibly lastModified changed
					result.Moved = append(result.Moved, docDiff)
				} else {
					result.Modified = append(result.Modified, docDiff)
				}
			} else {
				// Hash is same, but check for metadata-only changes
				changes := make(map[string]FieldChange)
				hasChanges := false
				isMoved := false

				if currentDoc.Metadata.DocName != previousDoc.Metadata.DocName {
					changes["name"] = FieldChange{
						Old: previousDoc.Metadata.DocName,
						New: currentDoc.Metadata.DocName,
					}
					hasChanges = true
				}
				if currentDoc.Metadata.Parent != previousDoc.Metadata.Parent {
					changes["parent"] = FieldChange{
						Old: previousDoc.Metadata.Parent,
						New: currentDoc.Metadata.Parent,
					}
					hasChanges = true
					isMoved = true
				}
				if currentDoc.Metadata.LastModified != previousDoc.Metadata.LastModified {
					changes["lastModified"] = FieldChange{
						Old: previousDoc.Metadata.LastModified,
						New: currentDoc.Metadata.LastModified,
					}
					hasChanges = true
				}
				if currentDoc.Metadata.Version != previousDoc.Metadata.Version {
					changes["version"] = FieldChange{
						Old: fmt.Sprintf("%d", previousDoc.Metadata.Version),
						New: fmt.Sprintf("%d", currentDoc.Metadata.Version),
					}
					hasChanges = true
				}
				if currentDoc.Metadata.Deleted != previousDoc.Metadata.Deleted {
					changes["deleted"] = FieldChange{
						Old: fmt.Sprintf("%v", previousDoc.Metadata.Deleted),
						New: fmt.Sprintf("%v", currentDoc.Metadata.Deleted),
					}
					hasChanges = true
				}
				if currentDoc.Metadata.Pinned != previousDoc.Metadata.Pinned {
					changes["pinned"] = FieldChange{
						Old: fmt.Sprintf("%v", previousDoc.Metadata.Pinned),
						New: fmt.Sprintf("%v", currentDoc.Metadata.Pinned),
					}
					hasChanges = true
				}

				if hasChanges {
					docDiff := DocDiff{
						DocumentID: id,
						Name:       currentDoc.Metadata.DocName,
						OldHash:    previousDoc.Hash,
						NewHash:    currentDoc.Hash,
						Changes:    changes,
					}
					
					// If only parent changed (moved without content change), categorize as moved
					// Otherwise, it's a metadata modification
					if isMoved && len(changes) == 1 {
						result.Moved = append(result.Moved, docDiff)
					} else {
						result.Modified = append(result.Modified, docDiff)
					}
				}
			}
		}
	}

	return result, nil
}

// LoadPreviousTree loads the previous tree from tree.cache.previous
func LoadPreviousTree() (*HashTree, error) {
	cacheFile, err := getCachedTreePath()
	if err != nil {
		return nil, err
	}

	backupFile := cacheFile + ".previous"
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("previous tree cache file does not exist: %s", backupFile)
	}

	data, err := os.ReadFile(backupFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read previous tree cache: %v", err)
	}

	tree := &HashTree{}
	if err := json.Unmarshal(data, tree); err != nil {
		return nil, fmt.Errorf("failed to parse previous tree cache: %v", err)
	}

	return tree, nil
}

// DiffTreeCache compares the current tree.cache with tree.cache.previous
func DiffTreeCache() (*TreeDiffResult, error) {
	currentTree, err := loadTree()
	if err != nil {
		return nil, fmt.Errorf("failed to load current tree: %v", err)
	}

	previousTree, err := LoadPreviousTree()
	if err != nil {
		return nil, fmt.Errorf("failed to load previous tree: %v", err)
	}

	result, err := DiffTrees(currentTree, previousTree)
	if err != nil {
		return nil, err
	}

	// Resolve paths using current and previous filetrees
	err = ResolvePaths(result, currentTree, previousTree)
	if err != nil {
		// Don't fail if path resolution fails, just log it
		fmt.Printf("Warning: failed to resolve some paths: %v\n", err)
	}

	return result, nil
}

// ResolvePaths resolves document IDs to their paths in the file tree
func ResolvePaths(result *TreeDiffResult, currentTree, previousTree *HashTree) error {
	// Build filetree from current tree for path resolution
	currentFileTree := DocumentsFileTree(currentTree)
	
	// Build filetree from previous tree for old paths
	previousFileTree := DocumentsFileTree(previousTree)

	// Resolve paths for modified documents
	for i := range result.Modified {
		docID := result.Modified[i].DocumentID
		if node := currentFileTree.NodeById(docID); node != nil {
			if path, err := currentFileTree.NodeToPath(node); err == nil {
				result.Modified[i].Path = path
			}
		}
		// Try to get old path from previous tree
		if node := previousFileTree.NodeById(docID); node != nil {
			if path, err := previousFileTree.NodeToPath(node); err == nil {
				result.Modified[i].OldPath = path
			}
		}
	}

	// Resolve paths for moved documents
	for i := range result.Moved {
		docID := result.Moved[i].DocumentID
		if node := currentFileTree.NodeById(docID); node != nil {
			if path, err := currentFileTree.NodeToPath(node); err == nil {
				result.Moved[i].Path = path
			}
		}
		// Get old path from previous tree
		if node := previousFileTree.NodeById(docID); node != nil {
			if path, err := previousFileTree.NodeToPath(node); err == nil {
				result.Moved[i].OldPath = path
			}
		}
	}

	// Resolve paths for new documents
	for _, doc := range result.New {
		if node := currentFileTree.NodeById(doc.DocumentID); node != nil {
			if path, err := currentFileTree.NodeToPath(node); err == nil {
				result.NewPaths[doc.DocumentID] = path
			}
		}
	}

	// Resolve paths for removed documents
	for _, doc := range result.Removed {
		if node := previousFileTree.NodeById(doc.DocumentID); node != nil {
			if path, err := previousFileTree.NodeToPath(node); err == nil {
				result.RemovedPaths[doc.DocumentID] = path
			}
		}
	}

	return nil
}


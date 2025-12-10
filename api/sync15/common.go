package sync15

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"sort"

	"github.com/juruen/rmapi/log"
)

func HashEntries(entries []*Entry) (string, error) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].DocumentID < entries[j].DocumentID })
	hasher := sha256.New()
	for _, d := range entries {
		//TODO: back and forth converting
		bh, err := hex.DecodeString(d.Hash)
		if err != nil {
			return "", err
		}
		hasher.Write(bh)
	}
	hash := hasher.Sum(nil)
	hashStr := hex.EncodeToString(hash)
	return hashStr, nil
}

func getCachedTreePath() (string, error) {
	cachedir, err := os.UserCacheDir()
	if err != nil {
		// Fallback to home directory if cache dir cannot be determined
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rmapiFolder := path.Join(home, ".rmapi-cache")
		if err := os.MkdirAll(rmapiFolder, 0700); err != nil {
			return "", err
		}
		cacheFile := path.Join(rmapiFolder, "tree.cache")
		return cacheFile, nil
	}
	rmapiFolder := path.Join(cachedir, "rmapi")
	err = os.MkdirAll(rmapiFolder, 0700)
	if err != nil {
		// Fallback to home directory if cache dir cannot be created
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rmapiFolder := path.Join(home, ".rmapi-cache")
		if err := os.MkdirAll(rmapiFolder, 0700); err != nil {
			return "", err
		}
		cacheFile := path.Join(rmapiFolder, "tree.cache")
		return cacheFile, nil
	}
	cacheFile := path.Join(rmapiFolder, "tree.cache")
	return cacheFile, nil
}

const cacheVersion = 3

func loadTree() (*HashTree, error) {
	cacheFile, err := getCachedTreePath()
	if err != nil {
		return nil, err
	}
	tree := &HashTree{}
	if _, err := os.Stat(cacheFile); err == nil {
		b, err := os.ReadFile(cacheFile)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(b, tree)
		if err != nil {
			log.Error.Println("cache corrupt, resyncing")
			return tree, nil
		}
		if tree.CacheVersion != cacheVersion {
			log.Info.Println("wrong cache file version, resyncing")
			return &HashTree{}, nil
		}
	}
	log.Info.Println("cache loaded: ", cacheFile)

	return tree, nil
}

// save cached version of the tree
func saveTree(tree *HashTree) error {
	cacheFile, err := getCachedTreePath()
	log.Info.Println("Writing cache: ", cacheFile)
	if err != nil {
		return err
	}
	tree.CacheVersion = cacheVersion
	b, err := json.MarshalIndent(tree, "", "")
	if err != nil {
		return err
	}
	err = os.WriteFile(cacheFile, b, 0644)
	return err
}

// DocumentSnapshot represents a snapshot of a document's state
type DocumentSnapshot struct {
	ID             string `json:"id"`
	Version        int    `json:"version"`
	ModifiedClient string `json:"modified_client"`
	Hash           string `json:"hash"`
}

// DiffSnapshot represents a snapshot of all documents
type DiffSnapshot struct {
	CacheVersion int                 `json:"cache_version"`
	Documents    []DocumentSnapshot  `json:"documents"`
	DocumentMap  map[string]DocumentSnapshot `json:"-"` // For quick lookup
}

const diffCacheVersion = 1

func getDiffSnapshotPath() (string, error) {
	cachedir, err := os.UserCacheDir()
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rmapiFolder := path.Join(home, ".rmapi-cache")
		if err := os.MkdirAll(rmapiFolder, 0700); err != nil {
			return "", err
		}
		cacheFile := path.Join(rmapiFolder, "diff.snapshot")
		return cacheFile, nil
	}
	rmapiFolder := path.Join(cachedir, "rmapi")
	err = os.MkdirAll(rmapiFolder, 0700)
	if err != nil {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		rmapiFolder := path.Join(home, ".rmapi-cache")
		if err := os.MkdirAll(rmapiFolder, 0700); err != nil {
			return "", err
		}
		cacheFile := path.Join(rmapiFolder, "diff.snapshot")
		return cacheFile, nil
	}
	cacheFile := path.Join(rmapiFolder, "diff.snapshot")
	return cacheFile, nil
}

func loadDiffSnapshot() (*DiffSnapshot, error) {
	cacheFile, err := getDiffSnapshotPath()
	if err != nil {
		return nil, err
	}
	snapshot := &DiffSnapshot{}
	if _, err := os.Stat(cacheFile); err == nil {
		b, err := os.ReadFile(cacheFile)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(b, snapshot)
		if err != nil {
			log.Error.Println("diff snapshot corrupt, starting fresh")
			return &DiffSnapshot{CacheVersion: diffCacheVersion, Documents: []DocumentSnapshot{}, DocumentMap: make(map[string]DocumentSnapshot)}, nil
		}
		if snapshot.CacheVersion != diffCacheVersion {
			log.Info.Println("wrong diff snapshot version, starting fresh")
			return &DiffSnapshot{CacheVersion: diffCacheVersion, Documents: []DocumentSnapshot{}, DocumentMap: make(map[string]DocumentSnapshot)}, nil
		}
		// Build map for quick lookup
		snapshot.DocumentMap = make(map[string]DocumentSnapshot)
		for _, doc := range snapshot.Documents {
			snapshot.DocumentMap[doc.ID] = doc
		}
	} else {
		// No snapshot exists yet
		snapshot = &DiffSnapshot{CacheVersion: diffCacheVersion, Documents: []DocumentSnapshot{}, DocumentMap: make(map[string]DocumentSnapshot)}
	}
	return snapshot, nil
}

func saveDiffSnapshot(tree *HashTree) error {
	cacheFile, err := getDiffSnapshotPath()
	if err != nil {
		return err
	}
	
	snapshot := &DiffSnapshot{
		CacheVersion: diffCacheVersion,
		Documents:    make([]DocumentSnapshot, 0, len(tree.Docs)),
		DocumentMap:  make(map[string]DocumentSnapshot),
	}
	
	for _, doc := range tree.Docs {
		if doc.Metadata.Deleted {
			continue
		}
		docSnapshot := DocumentSnapshot{
			ID:             doc.DocumentID,
			Hash:           doc.Hash,
		}
		
		// Extract version and modified time from metadata
		docSnapshot.Version = doc.Metadata.Version
		docSnapshot.ModifiedClient = doc.Metadata.LastModified
		
		snapshot.Documents = append(snapshot.Documents, docSnapshot)
		snapshot.DocumentMap[doc.DocumentID] = docSnapshot
	}
	
	b, err := json.MarshalIndent(snapshot, "", "")
	if err != nil {
		return err
	}
	err = os.WriteFile(cacheFile, b, 0644)
	return err
}

// DiffResult represents the result of comparing current state with snapshot
type DiffResult struct {
	HasChanges bool     `json:"has_changes"`
	NewFiles   []string `json:"new_files"`
	Modified   []string `json:"modified"`
	Deleted    []string `json:"deleted"`
}

func computeDiff(tree *HashTree, snapshot *DiffSnapshot) *DiffResult {
	result := &DiffResult{
		HasChanges: false,
		NewFiles:   []string{},
		Modified:   []string{},
		Deleted:    []string{},
	}
	
	currentDocs := make(map[string]*BlobDoc)
	for _, doc := range tree.Docs {
		if doc.Metadata.Deleted {
			continue
		}
		currentDocs[doc.DocumentID] = doc
	}
	
	// Check for new and modified files
	for id, doc := range currentDocs {
		prevSnapshot, exists := snapshot.DocumentMap[id]
		if !exists {
			// New file
			result.NewFiles = append(result.NewFiles, id)
			result.HasChanges = true
		} else {
			// Check if modified (version changed, hash changed, or modified time changed)
			if doc.Metadata.Version != prevSnapshot.Version ||
				doc.Hash != prevSnapshot.Hash ||
				doc.Metadata.LastModified != prevSnapshot.ModifiedClient {
				result.Modified = append(result.Modified, id)
				result.HasChanges = true
			}
		}
	}
	
	// Check for deleted files
	for id := range snapshot.DocumentMap {
		if _, exists := currentDocs[id]; !exists {
			result.Deleted = append(result.Deleted, id)
			result.HasChanges = true
		}
	}
	
	return result
}

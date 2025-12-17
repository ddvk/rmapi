package sync15

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// backupTreeCache creates a backup of the current tree.cache file as tree.cache.previous
func backupTreeCache() error {
	cacheFile, err := getCachedTreePath()
	if err != nil {
		return err
	}
	
	// Check if cache file exists
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		// No cache file to backup, skip silently
		return nil
	}
	
	backupFile := cacheFile + ".previous"
	
	// Read current cache file
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return fmt.Errorf("failed to read cache file for backup: %v", err)
	}
	
	// Write backup file
	err = os.WriteFile(backupFile, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write backup file: %v", err)
	}
	
	log.Info.Println("Backed up tree cache to: ", backupFile)
	return nil
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


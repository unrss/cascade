package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
)

// cacheEntry is the on-disk format for cached evaluation results.
type cacheEntry struct {
	Timestamp    time.Time `json:"timestamp"`
	RCPath       string    `json:"rc_path"` // For debugging
	Result       env.Env   `json:"result"`
	ExtraWatches []string  `json:"extra_watches,omitempty"`
}

// Cache stores evaluated .envrc results to avoid re-execution.
// Each entry is stored as a JSON file in the cache directory.
type Cache struct {
	dir string // e.g., ~/.cache/cascade/
}

// NewCache creates a cache using XDG_CACHE_HOME or ~/.cache/cascade.
func NewCache() (*Cache, error) {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache")
	}

	dir := filepath.Join(cacheDir, "cascade")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	return &Cache{dir: dir}, nil
}

// CacheKey computes a unique key for an evaluation.
// Key = SHA256(rc.ContentHash + inputEnvHash)
// This ensures cache invalidates when either the file OR input env changes.
func CacheKey(rc *envrc.RC, inputEnv env.Env) string {
	h := sha256.New()

	// Include the RC content hash (which already includes the file path)
	h.Write([]byte(rc.ContentHash))
	h.Write([]byte("\n"))

	// Include a hash of the input environment
	// ToGoEnv returns sorted keys for deterministic output
	for _, entry := range inputEnv.ToGoEnv() {
		h.Write([]byte(entry))
		h.Write([]byte("\x00"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Get retrieves a cached result if valid.
// Returns nil, false if not cached.
func (c *Cache) Get(key string) (*Result, bool) {
	path := c.entryPath(key)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false
		}
		// Other errors (permission, etc.) - treat as cache miss
		return nil, false
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupted cache entry - treat as miss
		return nil, false
	}

	return &Result{
		Env:          entry.Result,
		ExtraWatches: entry.ExtraWatches,
	}, true
}

// Set stores an evaluation result.
func (c *Cache) Set(key string, result *Result, rcPath string) error {
	entry := cacheEntry{
		Timestamp:    time.Now(),
		RCPath:       rcPath,
		Result:       result.Env,
		ExtraWatches: result.ExtraWatches,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache entry: %w", err)
	}

	path := c.entryPath(key)

	// Write atomically via temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write cache entry: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename cache entry: %w", err)
	}

	return nil
}

// Clear removes all cached entries.
func (c *Cache) Clear() error {
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read cache directory: %w", err)
	}

	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Only remove .json files to be safe
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.dir, entry.Name())
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to remove %d cache entries", len(errs))
	}
	return nil
}

// entryPath returns the file path for a cache key.
func (c *Cache) entryPath(key string) string {
	return filepath.Join(c.dir, key+".json")
}

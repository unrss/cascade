// Package state manages persistent environment state for cascade.
// State files are stored in ~/.local/share/cascade/state/ (or $XDG_DATA_HOME/cascade/state/).
package state

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
)

// Store manages persistent environment state for cascade.
type Store struct {
	dir string
}

// DirState represents the saved state for a single .envrc file.
type DirState struct {
	Path        string       `json:"path"` // Absolute .envrc path
	ContentHash string       `json:"hash"` // Content hash when saved
	Diff        *env.EnvDiff `json:"diff"` // Applied diff
	Timestamp   time.Time    `json:"ts"`   // Save time
}

// NewStore creates a state store, creating the directory if needed.
// Uses $XDG_DATA_HOME/cascade/state/ or ~/.local/share/cascade/state/.
func NewStore() (*Store, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	stateDir := filepath.Join(dataHome, "cascade", "state")
	return NewStoreWithDir(stateDir)
}

// NewStoreWithDir creates a Store with a custom directory (for testing).
func NewStoreWithDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Save persists the diff applied for an .envrc file.
// Uses path hash as filename: <state-dir>/<sha256(path)>.json
func (s *Store) Save(rcPath string, contentHash string, diff *env.EnvDiff) error {
	absPath, err := filepath.Abs(rcPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	state := &DirState{
		Path:        absPath,
		ContentHash: contentHash,
		Diff:        diff,
		Timestamp:   time.Now(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	pathHash := hashPath(absPath)
	stateFile := filepath.Join(s.dir, pathHash+".json")
	tmpFile := stateFile + ".tmp"

	// Atomic write: write to temp file, then rename
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, stateFile); err != nil {
		// Clean up temp file on rename failure
		_ = os.Remove(tmpFile)
		return fmt.Errorf("rename state file: %w", err)
	}

	return nil
}

// Load retrieves the last saved state for an .envrc path.
// Returns nil, nil if no state file exists (not an error).
func (s *Store) Load(rcPath string) (*DirState, error) {
	absPath, err := filepath.Abs(rcPath)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	pathHash := hashPath(absPath)
	stateFile := filepath.Join(s.dir, pathHash+".json")

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state DirState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// Delete removes the state file for an .envrc path.
// Returns nil if file doesn't exist.
func (s *Store) Delete(rcPath string) error {
	absPath, err := filepath.Abs(rcPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	pathHash := hashPath(absPath)
	stateFile := filepath.Join(s.dir, pathHash+".json")

	if err := os.Remove(stateFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove state file: %w", err)
	}

	return nil
}

// hashPath computes SHA256 of the absolute path.
func hashPath(absPath string) string {
	h := sha256.New()
	h.Write([]byte(absPath))
	return hex.EncodeToString(h.Sum(nil))
}

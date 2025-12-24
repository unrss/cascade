// Package envrc provides abstractions for .envrc file discovery and management.
package envrc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// RC represents a single .envrc file.
type RC struct {
	Path        string // Absolute path to .envrc
	Dir         string // Directory containing the .envrc
	Exists      bool   // Whether the file currently exists
	ContentHash string // SHA256(absolutePath + "\n" + content), empty if !Exists
}

// NewRC creates an RC from a path, computing hash if file exists.
// The path is resolved to an absolute path and symlinks are evaluated.
func NewRC(path string) (*RC, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("absolute path: %w", err)
	}

	// Check if file exists before resolving symlinks
	info, err := os.Lstat(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &RC{
				Path:   absPath,
				Dir:    filepath.Dir(absPath),
				Exists: false,
			}, nil
		}
		return nil, fmt.Errorf("stat %s: %w", absPath, err)
	}

	// Resolve symlinks for the actual file path
	resolvedPath := absPath
	if info.Mode()&os.ModeSymlink != 0 {
		resolvedPath, err = filepath.EvalSymlinks(absPath)
		if err != nil {
			return nil, fmt.Errorf("resolve symlink %s: %w", absPath, err)
		}
	}

	hash, err := fileHash(resolvedPath)
	if err != nil {
		return nil, err
	}

	return &RC{
		Path:        absPath,
		Dir:         filepath.Dir(absPath),
		Exists:      true,
		ContentHash: hash,
	}, nil
}

// Content returns the file content. Returns an error if the file does not exist.
func (rc *RC) Content() ([]byte, error) {
	if !rc.Exists {
		return nil, fmt.Errorf("file does not exist: %s", rc.Path)
	}
	return os.ReadFile(rc.Path)
}

// fileHash computes SHA256 of (absolute path + "\n" + content).
// This prevents both content modification AND symlink attacks.
func fileHash(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file %s: %w", path, err)
	}

	h := sha256.New()
	h.Write([]byte(path))
	h.Write([]byte("\n"))
	h.Write(content)

	return hex.EncodeToString(h.Sum(nil)), nil
}

// PathHash computes SHA256 of just the absolute path (for deny files).
func PathHash(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	// Resolve symlinks if the path exists
	if _, err := os.Lstat(absPath); err == nil {
		resolved, err := filepath.EvalSymlinks(absPath)
		if err == nil {
			absPath = resolved
		}
	}

	h := sha256.New()
	h.Write([]byte(absPath))

	return hex.EncodeToString(h.Sum(nil)), nil
}

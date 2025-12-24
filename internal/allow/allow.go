// Package allow manages the allow/deny security system for RC files.
package allow

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/unrss/cascade/internal/envrc"
)

// AllowStatus represents the authorization state of an RC file.
type AllowStatus int

const (
	Allowed    AllowStatus = iota // Explicitly allowed (content hash matches)
	NotAllowed                    // Not yet allowed (needs user approval)
	Denied                        // Explicitly denied
)

func (s AllowStatus) String() string {
	switch s {
	case Allowed:
		return "allowed"
	case NotAllowed:
		return "not allowed"
	case Denied:
		return "denied"
	default:
		return fmt.Sprintf("AllowStatus(%d)", s)
	}
}

// Store manages allow/deny state for RC files.
type Store struct {
	allowDir string // ~/.local/share/cascade/allow/
	denyDir  string // ~/.local/share/cascade/deny/
	trustDir string // ~/.local/share/cascade/trust/
}

// NewStore creates a Store with XDG-compliant paths.
// Uses $XDG_DATA_HOME/cascade/ or ~/.local/share/cascade/.
func NewStore() (*Store, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	baseDir := filepath.Join(dataHome, "cascade")
	return NewStoreWithBase(baseDir), nil
}

// NewStoreWithBase creates a Store with a custom base directory (for testing).
func NewStoreWithBase(baseDir string) *Store {
	return &Store{
		allowDir: filepath.Join(baseDir, "allow"),
		denyDir:  filepath.Join(baseDir, "deny"),
		trustDir: filepath.Join(baseDir, "trust"),
	}
}

// Whitelister checks if a path is whitelisted for auto-allow.
type Whitelister interface {
	IsWhitelisted(path string) bool
}

// Check returns the AllowStatus for an RC file.
// - Denied if deny file exists (keyed by path hash)
// - Allowed if allow file exists (keyed by content hash)
// - NotAllowed otherwise
func (s *Store) Check(rc *envrc.RC) AllowStatus {
	return s.CheckWithWhitelist(rc, nil)
}

// CheckWithWhitelist returns the AllowStatus for an RC file, considering whitelist.
// Priority: Denied > Allowed > TrustedSubtree > Whitelisted > NotAllowed
// - Denied if deny file exists (keyed by path hash) - takes precedence over everything
// - Allowed if allow file exists (keyed by content hash)
// - Allowed if path is under a trusted subtree
// - Allowed if path is whitelisted (config-based)
// - NotAllowed otherwise
func (s *Store) CheckWithWhitelist(rc *envrc.RC, wl Whitelister) AllowStatus {
	// Check deny first (path-based, takes precedence over everything)
	pathHash, err := envrc.PathHash(rc.Path)
	if err == nil {
		denyFile := filepath.Join(s.denyDir, pathHash)
		if _, err := os.Stat(denyFile); err == nil {
			return Denied
		}
	}

	// Check explicit allow (content-based)
	if rc.ContentHash != "" {
		allowFile := filepath.Join(s.allowDir, rc.ContentHash)
		if _, err := os.Stat(allowFile); err == nil {
			return Allowed
		}
	}

	// Check trusted subtree (path-based)
	if s.IsTrustedSubtree(rc.Path) {
		return Allowed
	}

	// Check whitelist (config-based, path prefix matching)
	if wl != nil && wl.IsWhitelisted(rc.Path) {
		return Allowed
	}

	return NotAllowed
}

// Allow marks an RC file as allowed.
// Creates allow file named by content hash, containing the path.
// Removes any existing deny file.
func (s *Store) Allow(rc *envrc.RC) error {
	if !rc.Exists {
		return fmt.Errorf("cannot allow non-existent file: %s", rc.Path)
	}

	if rc.ContentHash == "" {
		return fmt.Errorf("cannot allow file without content hash: %s", rc.Path)
	}

	// Create allow directory if needed
	if err := os.MkdirAll(s.allowDir, 0755); err != nil {
		return fmt.Errorf("create allow directory: %w", err)
	}

	// Write allow file
	allowFile := filepath.Join(s.allowDir, rc.ContentHash)
	if err := os.WriteFile(allowFile, []byte(rc.Path), 0644); err != nil {
		return fmt.Errorf("write allow file: %w", err)
	}

	// Remove any existing deny file
	pathHash, err := envrc.PathHash(rc.Path)
	if err != nil {
		return fmt.Errorf("compute path hash: %w", err)
	}

	denyFile := filepath.Join(s.denyDir, pathHash)
	if err := os.Remove(denyFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove deny file: %w", err)
	}

	return nil
}

// Deny marks an RC file as denied.
// Creates deny file named by path hash, containing the path.
// Removes any existing allow file.
func (s *Store) Deny(rc *envrc.RC) error {
	pathHash, err := envrc.PathHash(rc.Path)
	if err != nil {
		return fmt.Errorf("compute path hash: %w", err)
	}

	// Create deny directory if needed
	if err := os.MkdirAll(s.denyDir, 0755); err != nil {
		return fmt.Errorf("create deny directory: %w", err)
	}

	// Write deny file
	denyFile := filepath.Join(s.denyDir, pathHash)
	if err := os.WriteFile(denyFile, []byte(rc.Path), 0644); err != nil {
		return fmt.Errorf("write deny file: %w", err)
	}

	// Remove any existing allow file
	if rc.ContentHash != "" {
		allowFile := filepath.Join(s.allowDir, rc.ContentHash)
		if err := os.Remove(allowFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove allow file: %w", err)
		}
	}

	return nil
}

// Revoke removes both allow and deny status (back to NotAllowed).
func (s *Store) Revoke(rc *envrc.RC) error {
	var errs []error

	// Remove allow file if content hash exists
	if rc.ContentHash != "" {
		allowFile := filepath.Join(s.allowDir, rc.ContentHash)
		if err := os.Remove(allowFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove allow file: %w", err))
		}
	}

	// Remove deny file
	pathHash, err := envrc.PathHash(rc.Path)
	if err != nil {
		errs = append(errs, fmt.Errorf("compute path hash: %w", err))
	} else {
		denyFile := filepath.Join(s.denyDir, pathHash)
		if err := os.Remove(denyFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove deny file: %w", err))
		}
	}

	return errors.Join(errs...)
}

// TrustSubtree marks a directory subtree as trusted.
// Files under this path are auto-allowed when first loaded.
// Creates a file in trustDir named by path hash, containing the absolute path.
func (s *Store) TrustSubtree(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Verify the path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", absPath)
	}

	// Create trust directory if needed
	if err := os.MkdirAll(s.trustDir, 0755); err != nil {
		return fmt.Errorf("create trust directory: %w", err)
	}

	// Compute hash of the path for the filename
	pathHash, err := dirPathHash(absPath)
	if err != nil {
		return fmt.Errorf("compute path hash: %w", err)
	}

	// Write trust file containing the path
	trustFile := filepath.Join(s.trustDir, pathHash)
	if err := os.WriteFile(trustFile, []byte(absPath), 0644); err != nil {
		return fmt.Errorf("write trust file: %w", err)
	}

	return nil
}

// UntrustSubtree removes subtree trust for a directory.
func (s *Store) UntrustSubtree(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	pathHash, err := dirPathHash(absPath)
	if err != nil {
		return fmt.Errorf("compute path hash: %w", err)
	}

	trustFile := filepath.Join(s.trustDir, pathHash)
	if err := os.Remove(trustFile); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("subtree not trusted: %s", absPath)
		}
		return fmt.Errorf("remove trust file: %w", err)
	}

	return nil
}

// IsTrustedSubtree checks if a path is under a trusted subtree.
func (s *Store) IsTrustedSubtree(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	trustedPaths, err := s.ListTrustedSubtrees()
	if err != nil {
		return false
	}

	for _, trusted := range trustedPaths {
		if isUnderPath(absPath, trusted) {
			return true
		}
	}

	return false
}

// ListTrustedSubtrees returns all trusted subtree paths.
func (s *Store) ListTrustedSubtrees() ([]string, error) {
	entries, err := os.ReadDir(s.trustDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read trust directory: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		trustFile := filepath.Join(s.trustDir, entry.Name())
		content, err := os.ReadFile(trustFile)
		if err != nil {
			continue // Skip unreadable files
		}

		paths = append(paths, string(content))
	}

	return paths, nil
}

// dirPathHash computes SHA256 of a directory path (for trust files).
func dirPathHash(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	h := sha256.New()
	h.Write([]byte(absPath))

	return hex.EncodeToString(h.Sum(nil)), nil
}

// isUnderPath checks if child is under or equal to parent directory.
func isUnderPath(child, parent string) bool {
	// Clean paths for consistent comparison
	child = filepath.Clean(child)
	parent = filepath.Clean(parent)

	// Exact match
	if child == parent {
		return true
	}

	// Check if child starts with parent + separator
	parentWithSep := parent + string(filepath.Separator)
	return len(child) > len(parentWithSep) && child[:len(parentWithSep)] == parentWithSep
}

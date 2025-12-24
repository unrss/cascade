package envrc

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestNewRC_ExistingFile(t *testing.T) {
	// Get absolute path to testdata
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}

	envrcPath := filepath.Join(testdata, "home", ".envrc")
	rc, err := NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	if !rc.Exists {
		t.Error("expected Exists=true for existing file")
	}

	if rc.Path != envrcPath {
		t.Errorf("Path = %q, want %q", rc.Path, envrcPath)
	}

	if rc.Dir != filepath.Dir(envrcPath) {
		t.Errorf("Dir = %q, want %q", rc.Dir, filepath.Dir(envrcPath))
	}

	if rc.ContentHash == "" {
		t.Error("expected non-empty ContentHash for existing file")
	}

	// Verify hash format (64 hex chars for SHA256)
	if len(rc.ContentHash) != 64 {
		t.Errorf("ContentHash length = %d, want 64", len(rc.ContentHash))
	}

	// Verify Content() works
	content, err := rc.Content()
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty content")
	}
}

func TestNewRC_NonExistentFile(t *testing.T) {
	rc, err := NewRC("/nonexistent/path/.envrc")
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	if rc.Exists {
		t.Error("expected Exists=false for non-existent file")
	}

	if rc.ContentHash != "" {
		t.Errorf("expected empty ContentHash, got %q", rc.ContentHash)
	}

	// Content() should error
	_, err = rc.Content()
	if err == nil {
		t.Error("expected error from Content() on non-existent file")
	}
}

func TestFileHash_IncludesPath(t *testing.T) {
	// Create two temp files with identical content but different paths
	dir := t.TempDir()

	content := []byte("export FOO=bar\n")

	file1 := filepath.Join(dir, "file1.envrc")
	file2 := filepath.Join(dir, "file2.envrc")

	if err := os.WriteFile(file1, content, 0o644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(file2, content, 0o644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	hash1, err := fileHash(file1)
	if err != nil {
		t.Fatalf("fileHash file1: %v", err)
	}

	hash2, err := fileHash(file2)
	if err != nil {
		t.Fatalf("fileHash file2: %v", err)
	}

	if hash1 == hash2 {
		t.Error("same content with different paths should produce different hashes")
	}

	// Verify hash is computed correctly
	h := sha256.New()
	h.Write([]byte(file1))
	h.Write([]byte("\n"))
	h.Write(content)
	expected := hex.EncodeToString(h.Sum(nil))

	if hash1 != expected {
		t.Errorf("hash1 = %q, want %q", hash1, expected)
	}
}

func TestPathHash(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(file, []byte("test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hash, err := PathHash(file)
	if err != nil {
		t.Fatalf("pathHash: %v", err)
	}

	// Verify hash format
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// pathHash resolves symlinks, so we need the resolved path
	resolvedFile, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	// Verify it's just the resolved path
	h := sha256.New()
	h.Write([]byte(resolvedFile))
	expected := hex.EncodeToString(h.Sum(nil))

	if hash != expected {
		t.Errorf("hash = %q, want %q", hash, expected)
	}
}

func TestFindChain_CorrectOrder(t *testing.T) {
	testdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}

	root := filepath.Join(testdata, "home")
	target := filepath.Join(testdata, "home", "work", "api")

	chain, err := FindChain(root, target)
	if err != nil {
		t.Fatalf("FindChain: %v", err)
	}

	// Should have 3 entries: home, work, api
	if len(chain) != 3 {
		t.Fatalf("len(chain) = %d, want 3", len(chain))
	}

	// Verify order: root first, target last
	expectedDirs := []string{
		filepath.Join(testdata, "home"),
		filepath.Join(testdata, "home", "work"),
		filepath.Join(testdata, "home", "work", "api"),
	}

	for i, rc := range chain {
		if rc.Dir != expectedDirs[i] {
			t.Errorf("chain[%d].Dir = %q, want %q", i, rc.Dir, expectedDirs[i])
		}
		if !rc.Exists {
			t.Errorf("chain[%d].Exists = false, want true", i)
		}
	}
}

func TestFindChain_HandlesSymlinks(t *testing.T) {
	dir := t.TempDir()

	// Create actual directory structure
	actualDir := filepath.Join(dir, "actual", "project")
	if err := os.MkdirAll(actualDir, 0o755); err != nil {
		t.Fatalf("mkdir actual: %v", err)
	}

	envrcPath := filepath.Join(actualDir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte("export X=1\n"), 0o644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	// Create symlink
	linkDir := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "actual"), linkDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// FindChain through symlink should resolve it
	chain, err := FindChain(linkDir, filepath.Join(linkDir, "project"))
	if err != nil {
		t.Fatalf("FindChain: %v", err)
	}

	// Should have 2 entries (actual, actual/project)
	if len(chain) != 2 {
		t.Fatalf("len(chain) = %d, want 2", len(chain))
	}

	// The project entry should exist and have a hash
	projectRC := chain[1]
	if !projectRC.Exists {
		t.Error("project RC should exist")
	}
	if projectRC.ContentHash == "" {
		t.Error("project RC should have hash")
	}
}

func TestFindChain_NoEnvrcFiles(t *testing.T) {
	dir := t.TempDir()

	// Create directory structure without any .envrc files
	subdir := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	chain, err := FindChain(dir, subdir)
	if err != nil {
		t.Fatalf("FindChain: %v", err)
	}

	// Should have 4 entries: dir, a, b, c
	if len(chain) != 4 {
		t.Fatalf("len(chain) = %d, want 4", len(chain))
	}

	// All should have Exists=false
	for i, rc := range chain {
		if rc.Exists {
			t.Errorf("chain[%d].Exists = true, want false", i)
		}
		if rc.ContentHash != "" {
			t.Errorf("chain[%d].ContentHash = %q, want empty", i, rc.ContentHash)
		}
	}
}

func TestFindChain_TargetNotUnderRoot(t *testing.T) {
	_, err := FindChain("/home/user", "/var/log")
	if err == nil {
		t.Error("expected error when target is not under root")
	}
}

func TestExistingOnly(t *testing.T) {
	chain := []*RC{
		{Path: "/a/.envrc", Exists: true},
		{Path: "/a/b/.envrc", Exists: false},
		{Path: "/a/b/c/.envrc", Exists: true},
	}

	existing := ExistingOnly(chain)

	if len(existing) != 2 {
		t.Fatalf("len(existing) = %d, want 2", len(existing))
	}

	if existing[0].Path != "/a/.envrc" {
		t.Errorf("existing[0].Path = %q, want /a/.envrc", existing[0].Path)
	}
	if existing[1].Path != "/a/b/c/.envrc" {
		t.Errorf("existing[1].Path = %q, want /a/b/c/.envrc", existing[1].Path)
	}
}

func TestNewRC_Symlink(t *testing.T) {
	dir := t.TempDir()

	// Resolve the temp dir itself (macOS /var -> /private/var)
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("eval symlinks dir: %v", err)
	}

	// Create actual file
	actualFile := filepath.Join(dir, "actual.envrc")
	content := []byte("export SYMLINK_TEST=1\n")
	if err := os.WriteFile(actualFile, content, 0o644); err != nil {
		t.Fatalf("write actual: %v", err)
	}

	// Create symlink
	linkFile := filepath.Join(dir, ".envrc")
	if err := os.Symlink(actualFile, linkFile); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rc, err := NewRC(linkFile)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	if !rc.Exists {
		t.Error("expected Exists=true for symlinked file")
	}

	// Hash should be based on resolved path, not symlink path
	// This prevents symlink attacks where content is the same but target differs
	h := sha256.New()
	h.Write([]byte(actualFile)) // resolved path
	h.Write([]byte("\n"))
	h.Write(content)
	expectedHash := hex.EncodeToString(h.Sum(nil))

	if rc.ContentHash != expectedHash {
		t.Errorf("ContentHash = %q, want %q (based on resolved path)", rc.ContentHash, expectedHash)
	}
}

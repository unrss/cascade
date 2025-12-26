package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/unrss/cascade/internal/env"
)

func TestNewStore_CreatesDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "cascade", "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	if store == nil {
		t.Fatal("NewStoreWithDir returned nil store")
	}

	// Verify directory was created
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}

	if !info.IsDir() {
		t.Error("state dir is not a directory")
	}

	// Verify permissions (0700)
	if info.Mode().Perm() != 0700 {
		t.Errorf("state dir permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestNewStore_UsesXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if store == nil {
		t.Fatal("NewStore returned nil store")
	}

	// Verify directory was created at XDG_DATA_HOME/cascade/state
	expectedDir := filepath.Join(dir, "cascade", "state")
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}

	if !info.IsDir() {
		t.Error("state dir is not a directory")
	}
}

func TestNewStore_FallsBackToLocalShare(t *testing.T) {
	// Create a temp home directory
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", "")

	store, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	if store == nil {
		t.Fatal("NewStore returned nil store")
	}

	// Verify directory was created at ~/.local/share/cascade/state
	expectedDir := filepath.Join(dir, ".local", "share", "cascade", "state")
	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}

	if !info.IsDir() {
		t.Error("state dir is not a directory")
	}
}

func TestNewStore_NestedDirectoryCreation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "deeply", "nested", "cascade", "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	if store == nil {
		t.Fatal("NewStoreWithDir returned nil store")
	}

	// Verify all nested directories were created
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatalf("stat state dir: %v", err)
	}

	if !info.IsDir() {
		t.Error("state dir is not a directory")
	}
}

func TestSave_WritesStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	contentHash := "abc123"
	diff := &env.EnvDiff{
		Prev: map[string]string{"OLD": "value"},
		Next: map[string]string{"NEW": "value"},
	}

	if err := store.Save(rcPath, contentHash, diff); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file was created
	absPath, _ := filepath.Abs(rcPath)
	pathHash := testHashPath(absPath)
	stateFile := filepath.Join(stateDir, pathHash+".json")

	info, err := os.Stat(stateFile)
	if err != nil {
		t.Fatalf("stat state file: %v", err)
	}

	// Verify permissions (0600)
	if info.Mode().Perm() != 0600 {
		t.Errorf("state file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Verify content
	data, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}

	var state DirState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}

	if state.Path != absPath {
		t.Errorf("state.Path = %q, want %q", state.Path, absPath)
	}

	if state.ContentHash != contentHash {
		t.Errorf("state.ContentHash = %q, want %q", state.ContentHash, contentHash)
	}

	if state.Diff == nil {
		t.Fatal("state.Diff is nil")
	}

	if state.Diff.Next["NEW"] != "value" {
		t.Errorf("state.Diff.Next[NEW] = %q, want %q", state.Diff.Next["NEW"], "value")
	}

	if state.Timestamp.IsZero() {
		t.Error("state.Timestamp is zero")
	}
}

func TestSave_AtomicWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	diff := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"FOO": "bar"},
	}

	// Save initial state
	if err := store.Save(rcPath, "hash1", diff); err != nil {
		t.Fatalf("Save initial: %v", err)
	}

	// Save updated state
	diff2 := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"FOO": "baz"},
	}
	if err := store.Save(rcPath, "hash2", diff2); err != nil {
		t.Fatalf("Save updated: %v", err)
	}

	// Verify no temp file left behind
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}

	// Verify updated content
	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state.ContentHash != "hash2" {
		t.Errorf("state.ContentHash = %q, want %q", state.ContentHash, "hash2")
	}

	if state.Diff.Next["FOO"] != "baz" {
		t.Errorf("state.Diff.Next[FOO] = %q, want %q", state.Diff.Next["FOO"], "baz")
	}
}

func TestLoad_ReturnsNilForMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	state, err := store.Load("/nonexistent/path/.envrc")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state != nil {
		t.Errorf("Load returned non-nil state for missing file: %+v", state)
	}
}

func TestLoad_ReadsStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	contentHash := "testhash"
	diff := &env.EnvDiff{
		Prev: map[string]string{"A": "1"},
		Next: map[string]string{"B": "2"},
	}

	if err := store.Save(rcPath, contentHash, diff); err != nil {
		t.Fatalf("Save: %v", err)
	}

	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state == nil {
		t.Fatal("Load returned nil state")
	}

	absPath, _ := filepath.Abs(rcPath)
	if state.Path != absPath {
		t.Errorf("state.Path = %q, want %q", state.Path, absPath)
	}

	if state.ContentHash != contentHash {
		t.Errorf("state.ContentHash = %q, want %q", state.ContentHash, contentHash)
	}

	if state.Diff.Prev["A"] != "1" {
		t.Errorf("state.Diff.Prev[A] = %q, want %q", state.Diff.Prev["A"], "1")
	}

	if state.Diff.Next["B"] != "2" {
		t.Errorf("state.Diff.Next[B] = %q, want %q", state.Diff.Next["B"], "2")
	}
}

func TestLoad_HandlesCorruptedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	// Create a corrupted state file
	rcPath := filepath.Join(dir, "project", ".envrc")
	absPath, _ := filepath.Abs(rcPath)
	pathHash := testHashPath(absPath)
	stateFile := filepath.Join(stateDir, pathHash+".json")

	if err := os.WriteFile(stateFile, []byte("not valid json{{{"), 0600); err != nil {
		t.Fatalf("write corrupted file: %v", err)
	}

	state, err := store.Load(rcPath)
	if err == nil {
		t.Error("Load should return error for corrupted file")
	}

	if state != nil {
		t.Errorf("Load returned non-nil state for corrupted file: %+v", state)
	}
}

func TestLoad_HandlesEmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	// Create an empty state file
	rcPath := filepath.Join(dir, "project", ".envrc")
	absPath, _ := filepath.Abs(rcPath)
	pathHash := testHashPath(absPath)
	stateFile := filepath.Join(stateDir, pathHash+".json")

	if err := os.WriteFile(stateFile, []byte(""), 0600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	state, err := store.Load(rcPath)
	if err == nil {
		t.Error("Load should return error for empty file")
	}

	if state != nil {
		t.Errorf("Load returned non-nil state for empty file: %+v", state)
	}
}

func TestDelete_RemovesStateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	diff := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"FOO": "bar"},
	}

	// Save state
	if err := store.Save(rcPath, "hash", diff); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify it exists
	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load before delete: %v", err)
	}
	if state == nil {
		t.Fatal("state should exist before delete")
	}

	// Delete
	if err := store.Delete(rcPath); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify it's gone
	state, err = store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if state != nil {
		t.Error("state should be nil after delete")
	}
}

func TestDelete_HandlesMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	// Delete non-existent file should not error
	err = store.Delete("/nonexistent/path/.envrc")
	if err != nil {
		t.Errorf("Delete non-existent file returned error: %v", err)
	}
}

func TestPathHashConsistency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
	}{
		{"simple path", "/home/user/project/.envrc"},
		{"nested path", "/home/user/work/company/project/subdir/.envrc"},
		{"path with spaces", "/home/user/my project/.envrc"},
		{"path with special chars", "/home/user/project-v2.0/.envrc"},
		{"root path", "/.envrc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hash1 := testHashPath(tt.path)
			hash2 := testHashPath(tt.path)

			if hash1 != hash2 {
				t.Errorf("hash inconsistent: %q != %q", hash1, hash2)
			}

			// Verify hash format (64 hex chars for SHA256)
			if len(hash1) != 64 {
				t.Errorf("hash length = %d, want 64", len(hash1))
			}
		})
	}
}

func TestPathHashUniqueness(t *testing.T) {
	t.Parallel()

	paths := []string{
		"/home/user/project1/.envrc",
		"/home/user/project2/.envrc",
		"/home/user/project/.envrc",
		"/home/other/project/.envrc",
	}

	hashes := make(map[string]string)
	for _, path := range paths {
		hash := testHashPath(path)
		if existing, ok := hashes[hash]; ok {
			t.Errorf("hash collision: %q and %q both hash to %q", path, existing, hash)
		}
		hashes[hash] = path
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	contentHash := "roundtriphash"
	diff := &env.EnvDiff{
		Prev: map[string]string{
			"REMOVED": "oldvalue",
			"CHANGED": "before",
		},
		Next: map[string]string{
			"ADDED":   "newvalue",
			"CHANGED": "after",
		},
	}

	if err := store.Save(rcPath, contentHash, diff); err != nil {
		t.Fatalf("Save: %v", err)
	}

	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state == nil {
		t.Fatal("Load returned nil state")
	}

	if state.ContentHash != contentHash {
		t.Errorf("ContentHash = %q, want %q", state.ContentHash, contentHash)
	}

	// Verify diff was preserved
	if state.Diff.Prev["REMOVED"] != "oldvalue" {
		t.Errorf("Diff.Prev[REMOVED] = %q, want %q", state.Diff.Prev["REMOVED"], "oldvalue")
	}
	if state.Diff.Prev["CHANGED"] != "before" {
		t.Errorf("Diff.Prev[CHANGED] = %q, want %q", state.Diff.Prev["CHANGED"], "before")
	}
	if state.Diff.Next["ADDED"] != "newvalue" {
		t.Errorf("Diff.Next[ADDED] = %q, want %q", state.Diff.Next["ADDED"], "newvalue")
	}
	if state.Diff.Next["CHANGED"] != "after" {
		t.Errorf("Diff.Next[CHANGED] = %q, want %q", state.Diff.Next["CHANGED"], "after")
	}
}

func TestSave_NilDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")

	// Save with nil diff
	if err := store.Save(rcPath, "hash", nil); err != nil {
		t.Fatalf("Save with nil diff: %v", err)
	}

	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state == nil {
		t.Fatal("Load returned nil state")
	}

	if state.Diff != nil {
		t.Errorf("expected nil diff, got %+v", state.Diff)
	}
}

func TestSave_EmptyDiff(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	rcPath := filepath.Join(dir, "project", ".envrc")
	diff := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{},
	}

	if err := store.Save(rcPath, "hash", diff); err != nil {
		t.Fatalf("Save with empty diff: %v", err)
	}

	state, err := store.Load(rcPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if state == nil {
		t.Fatal("Load returned nil state")
	}

	if state.Diff == nil {
		t.Fatal("expected non-nil diff")
	}

	if len(state.Diff.Prev) != 0 {
		t.Errorf("expected empty Prev, got %v", state.Diff.Prev)
	}

	if len(state.Diff.Next) != 0 {
		t.Errorf("expected empty Next, got %v", state.Diff.Next)
	}
}

func TestMultipleStates_Independent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	// Save states for two different paths
	rcPath1 := filepath.Join(dir, "project1", ".envrc")
	rcPath2 := filepath.Join(dir, "project2", ".envrc")

	diff1 := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"PROJECT": "one"},
	}
	diff2 := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"PROJECT": "two"},
	}

	if err := store.Save(rcPath1, "hash1", diff1); err != nil {
		t.Fatalf("Save path1: %v", err)
	}
	if err := store.Save(rcPath2, "hash2", diff2); err != nil {
		t.Fatalf("Save path2: %v", err)
	}

	// Load and verify they're independent
	state1, err := store.Load(rcPath1)
	if err != nil {
		t.Fatalf("Load path1: %v", err)
	}
	state2, err := store.Load(rcPath2)
	if err != nil {
		t.Fatalf("Load path2: %v", err)
	}

	if state1.Diff.Next["PROJECT"] != "one" {
		t.Errorf("state1.Diff.Next[PROJECT] = %q, want %q", state1.Diff.Next["PROJECT"], "one")
	}
	if state2.Diff.Next["PROJECT"] != "two" {
		t.Errorf("state2.Diff.Next[PROJECT] = %q, want %q", state2.Diff.Next["PROJECT"], "two")
	}

	// Delete one, verify other still exists
	if err := store.Delete(rcPath1); err != nil {
		t.Fatalf("Delete path1: %v", err)
	}

	state1, err = store.Load(rcPath1)
	if err != nil {
		t.Fatalf("Load path1 after delete: %v", err)
	}
	if state1 != nil {
		t.Error("state1 should be nil after delete")
	}

	state2, err = store.Load(rcPath2)
	if err != nil {
		t.Fatalf("Load path2 after delete of path1: %v", err)
	}
	if state2 == nil {
		t.Error("state2 should still exist")
	}
}

func TestRelativePath_ResolvedToAbsolute(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	store, err := NewStoreWithDir(stateDir)
	if err != nil {
		t.Fatalf("NewStoreWithDir: %v", err)
	}

	// Create a file in the temp dir
	envrcPath := filepath.Join(dir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	// Use t.Chdir for safe directory change
	t.Chdir(dir)

	diff := &env.EnvDiff{
		Prev: map[string]string{},
		Next: map[string]string{"FOO": "bar"},
	}

	// Save with relative path
	if err := store.Save(".envrc", "hash", diff); err != nil {
		t.Fatalf("Save with relative path: %v", err)
	}

	// Load with absolute path should find it
	state, err := store.Load(envrcPath)
	if err != nil {
		t.Fatalf("Load with absolute path: %v", err)
	}

	if state == nil {
		t.Error("state should be found when loading with absolute path")
	}
}

// testHashPath is a test helper that mirrors the internal hashPath function.
func testHashPath(absPath string) string {
	h := sha256.New()
	h.Write([]byte(absPath))
	return hex.EncodeToString(h.Sum(nil))
}

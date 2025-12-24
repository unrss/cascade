package env

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileTime_ExistingFile(t *testing.T) {
	// Create a temp file
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ft := NewFileTime(f.Name())

	if !ft.Exists {
		t.Error("Exists should be true for existing file")
	}
	if ft.Path != f.Name() {
		t.Errorf("Path = %q, want %q", ft.Path, f.Name())
	}
	if ft.Modtime == 0 {
		t.Error("Modtime should be non-zero for existing file")
	}

	// Verify modtime is reasonable (within last minute)
	now := time.Now().Unix()
	if ft.Modtime < now-60 || ft.Modtime > now+1 {
		t.Errorf("Modtime %d not within expected range of %d", ft.Modtime, now)
	}
}

func TestNewFileTime_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist")

	ft := NewFileTime(path)

	if ft.Exists {
		t.Error("Exists should be false for non-existent file")
	}
	if ft.Path != path {
		t.Errorf("Path = %q, want %q", ft.Path, path)
	}
	if ft.Modtime != 0 {
		t.Errorf("Modtime = %d, want 0 for non-existent file", ft.Modtime)
	}
}

func TestFileTime_Check_NoChange(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ft := NewFileTime(f.Name())

	if ft.Check() {
		t.Error("Check should return false when file unchanged")
	}
}

func TestFileTime_Check_Modification(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ft := NewFileTime(f.Name())

	// Modify the file's modtime (add 2 seconds to ensure change)
	newTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(f.Name(), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if !ft.Check() {
		t.Error("Check should return true when file modified")
	}
}

func TestFileTime_Check_Creation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "will-be-created")

	// Start with non-existent file
	ft := NewFileTime(path)
	if ft.Exists {
		t.Fatal("precondition: file should not exist")
	}

	// Create the file
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !ft.Check() {
		t.Error("Check should return true when file created")
	}
}

func TestFileTime_Check_Deletion(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	ft := NewFileTime(f.Name())
	if !ft.Exists {
		t.Fatal("precondition: file should exist")
	}

	// Delete the file
	if err := os.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}

	if !ft.Check() {
		t.Error("Check should return true when file deleted")
	}
}

func TestWatchList_Check_NoChanges(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "a"),
		filepath.Join(dir, "b"),
	}

	// Create files
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	wl := NewWatchList(paths)

	if wl.Check() {
		t.Error("Check should return false when no files changed")
	}
}

func TestWatchList_Check_OneChanged(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "a"),
		filepath.Join(dir, "b"),
		filepath.Join(dir, "c"),
	}

	// Create files
	for _, p := range paths {
		if err := os.WriteFile(p, []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	wl := NewWatchList(paths)

	// Modify middle file
	newTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(paths[1], newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if !wl.Check() {
		t.Error("Check should return true when any file changed")
	}
}

func TestWatchList_Empty(t *testing.T) {
	wl := NewWatchList(nil)

	if wl.Check() {
		t.Error("empty WatchList.Check should return false")
	}

	encoded, err := wl.Serialize()
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	if encoded != "" {
		t.Errorf("empty WatchList should serialize to empty string, got %q", encoded)
	}
}

func TestWatchList_SerializeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "file1"),
		filepath.Join(dir, "file2"),
	}

	// Create one file, leave other non-existent
	if err := os.WriteFile(paths[0], []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := NewWatchList(paths)

	encoded, err := original.Serialize()
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}

	if encoded == "" {
		t.Fatal("Serialize returned empty string for non-empty WatchList")
	}

	decoded, err := ParseWatchList(encoded)
	if err != nil {
		t.Fatalf("ParseWatchList error: %v", err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(original))
	}

	for i := range original {
		if decoded[i].Path != original[i].Path {
			t.Errorf("[%d] Path = %q, want %q", i, decoded[i].Path, original[i].Path)
		}
		if decoded[i].Modtime != original[i].Modtime {
			t.Errorf("[%d] Modtime = %d, want %d", i, decoded[i].Modtime, original[i].Modtime)
		}
		if decoded[i].Exists != original[i].Exists {
			t.Errorf("[%d] Exists = %v, want %v", i, decoded[i].Exists, original[i].Exists)
		}
	}
}

func TestParseWatchList_Empty(t *testing.T) {
	wl, err := ParseWatchList("")
	if err != nil {
		t.Fatalf("ParseWatchList error: %v", err)
	}
	if len(wl) != 0 {
		t.Errorf("expected empty WatchList, got %d items", len(wl))
	}
}

func TestParseWatchList_InvalidBase64(t *testing.T) {
	_, err := ParseWatchList("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestParseWatchList_InvalidZlib(t *testing.T) {
	// Valid base64 but not zlib data
	_, err := ParseWatchList("aGVsbG8gd29ybGQ=")
	if err == nil {
		t.Error("expected error for invalid zlib data")
	}
}

func TestNewFileTime_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")

	// Create target file
	if err := os.WriteFile(target, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create symlink
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	ftTarget := NewFileTime(target)
	ftLink := NewFileTime(link)

	// Both should exist and have same modtime (os.Stat follows symlinks)
	if !ftLink.Exists {
		t.Error("symlink should report as existing")
	}
	if ftLink.Modtime != ftTarget.Modtime {
		t.Errorf("symlink modtime = %d, target modtime = %d", ftLink.Modtime, ftTarget.Modtime)
	}
}

package allow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unrss/cascade/internal/envrc"
)

func TestCheck_NewFile_ReturnsNotAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)
	status := store.Check(rc)

	if status != NotAllowed {
		t.Errorf("Check() = %v, want NotAllowed", status)
	}
}

func TestAllow_ThenCheck_ReturnsAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	status := store.Check(rc)
	if status != Allowed {
		t.Errorf("Check() = %v, want Allowed", status)
	}
}

func TestDeny_ThenCheck_ReturnsDenied(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	status := store.Check(rc)
	if status != Denied {
		t.Errorf("Check() = %v, want Denied", status)
	}
}

func TestAllow_RemovesExistingDeny(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// First deny
	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	if status := store.Check(rc); status != Denied {
		t.Fatalf("after Deny, Check() = %v, want Denied", status)
	}

	// Then allow (should remove deny)
	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	status := store.Check(rc)
	if status != Allowed {
		t.Errorf("after Allow, Check() = %v, want Allowed", status)
	}
}

func TestDeny_RemovesExistingAllow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// First allow
	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	if status := store.Check(rc); status != Allowed {
		t.Fatalf("after Allow, Check() = %v, want Allowed", status)
	}

	// Then deny (should remove allow)
	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	status := store.Check(rc)
	if status != Denied {
		t.Errorf("after Deny, Check() = %v, want Denied", status)
	}
}

func TestContentChange_InvalidatesAllow(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	// Create initial file
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc1, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Allow the file
	if err := store.Allow(rc1); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	if status := store.Check(rc1); status != Allowed {
		t.Fatalf("after Allow, Check() = %v, want Allowed", status)
	}

	// Modify the file content
	if err := os.WriteFile(envrcPath, []byte("export FOO=malicious"), 0644); err != nil {
		t.Fatalf("write modified envrc: %v", err)
	}

	// Create new RC with updated content hash
	rc2, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC after modification: %v", err)
	}

	// Verify hashes are different
	if rc1.ContentHash == rc2.ContentHash {
		t.Fatal("content hashes should differ after modification")
	}

	// Modified file should not be allowed
	status := store.Check(rc2)
	if status != NotAllowed {
		t.Errorf("after content change, Check() = %v, want NotAllowed", status)
	}
}

func TestPathChange_SameContent_NotAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	content := []byte("export FOO=bar")

	// Create first file
	envrcPath1 := filepath.Join(dir, "project1", ".envrc")
	if err := os.MkdirAll(filepath.Dir(envrcPath1), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath1, content, 0644); err != nil {
		t.Fatalf("write envrc1: %v", err)
	}

	// Create second file with same content but different path
	envrcPath2 := filepath.Join(dir, "project2", ".envrc")
	if err := os.MkdirAll(filepath.Dir(envrcPath2), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath2, content, 0644); err != nil {
		t.Fatalf("write envrc2: %v", err)
	}

	rc1, err := envrc.NewRC(envrcPath1)
	if err != nil {
		t.Fatalf("NewRC for path1: %v", err)
	}

	rc2, err := envrc.NewRC(envrcPath2)
	if err != nil {
		t.Fatalf("NewRC for path2: %v", err)
	}

	// Verify content hashes are different (path is included in hash)
	if rc1.ContentHash == rc2.ContentHash {
		t.Fatal("content hashes should differ for different paths")
	}

	store := NewStoreWithBase(storeDir)

	// Allow first file
	if err := store.Allow(rc1); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	// First file should be allowed
	if status := store.Check(rc1); status != Allowed {
		t.Errorf("Check(rc1) = %v, want Allowed", status)
	}

	// Second file (same content, different path) should NOT be allowed
	status := store.Check(rc2)
	if status != NotAllowed {
		t.Errorf("Check(rc2) = %v, want NotAllowed", status)
	}
}

func TestRevoke_RemovesBothAllowAndDeny(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Test revoking allow
	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if status := store.Check(rc); status != Allowed {
		t.Fatalf("after Allow, Check() = %v, want Allowed", status)
	}

	if err := store.Revoke(rc); err != nil {
		t.Fatalf("Revoke after Allow: %v", err)
	}
	if status := store.Check(rc); status != NotAllowed {
		t.Errorf("after Revoke, Check() = %v, want NotAllowed", status)
	}

	// Test revoking deny
	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}
	if status := store.Check(rc); status != Denied {
		t.Fatalf("after Deny, Check() = %v, want Denied", status)
	}

	if err := store.Revoke(rc); err != nil {
		t.Fatalf("Revoke after Deny: %v", err)
	}
	if status := store.Check(rc); status != NotAllowed {
		t.Errorf("after Revoke, Check() = %v, want NotAllowed", status)
	}
}

func TestAllowStatus_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status AllowStatus
		want   string
	}{
		{Allowed, "allowed"},
		{NotAllowed, "not allowed"},
		{Denied, "denied"},
		{AllowStatus(99), "AllowStatus(99)"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// mockWhitelister implements Whitelister for testing.
type mockWhitelister struct {
	prefixes []string
}

func (m *mockWhitelister) IsWhitelisted(path string) bool {
	for _, prefix := range m.prefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			// Check directory boundary
			if len(path) == len(prefix) || path[len(prefix)] == '/' {
				return true
			}
		}
	}
	return false
}

func TestCheckWithWhitelist_WhitelistedPath_ReturnsAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)
	wl := &mockWhitelister{prefixes: []string{trustedDir}}

	// Without whitelist, should be NotAllowed
	status := store.Check(rc)
	if status != NotAllowed {
		t.Errorf("Check() = %v, want NotAllowed", status)
	}

	// With whitelist, should be Allowed
	status = store.CheckWithWhitelist(rc, wl)
	if status != Allowed {
		t.Errorf("CheckWithWhitelist() = %v, want Allowed", status)
	}
}

func TestCheckWithWhitelist_DenyTakesPrecedence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)
	wl := &mockWhitelister{prefixes: []string{trustedDir}}

	// Deny the file
	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	// Even with whitelist, deny should take precedence
	status := store.CheckWithWhitelist(rc, wl)
	if status != Denied {
		t.Errorf("CheckWithWhitelist() = %v, want Denied (deny takes precedence)", status)
	}
}

func TestCheckWithWhitelist_NilWhitelister(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	envrcPath := filepath.Join(dir, ".envrc")

	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// With nil whitelister, should behave like Check()
	status := store.CheckWithWhitelist(rc, nil)
	if status != NotAllowed {
		t.Errorf("CheckWithWhitelist(nil) = %v, want NotAllowed", status)
	}

	// Allow the file
	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	status = store.CheckWithWhitelist(rc, nil)
	if status != Allowed {
		t.Errorf("CheckWithWhitelist(nil) after Allow = %v, want Allowed", status)
	}
}

func TestTrustSubtree_ThenCheck_ReturnsAllowed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, "project", ".envrc")

	// Create nested directory structure
	if err := os.MkdirAll(filepath.Dir(envrcPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Before trusting, should be NotAllowed
	status := store.Check(rc)
	if status != NotAllowed {
		t.Errorf("before trust, Check() = %v, want NotAllowed", status)
	}

	// Trust the parent directory
	if err := store.TrustSubtree(trustedDir); err != nil {
		t.Fatalf("TrustSubtree: %v", err)
	}

	// After trusting, should be Allowed
	status = store.Check(rc)
	if status != Allowed {
		t.Errorf("after trust, Check() = %v, want Allowed", status)
	}
}

func TestTrustSubtree_RequiresDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	filePath := filepath.Join(dir, "somefile")

	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	err := store.TrustSubtree(filePath)
	if err == nil {
		t.Error("TrustSubtree(file) should return error")
	}
}

func TestUntrustSubtree_RemovesTrust(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Trust and verify
	if err := store.TrustSubtree(trustedDir); err != nil {
		t.Fatalf("TrustSubtree: %v", err)
	}
	if status := store.Check(rc); status != Allowed {
		t.Fatalf("after trust, Check() = %v, want Allowed", status)
	}

	// Untrust and verify
	if err := store.UntrustSubtree(trustedDir); err != nil {
		t.Fatalf("UntrustSubtree: %v", err)
	}
	if status := store.Check(rc); status != NotAllowed {
		t.Errorf("after untrust, Check() = %v, want NotAllowed", status)
	}
}

func TestUntrustSubtree_NotTrusted_ReturnsError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	untrustedDir := filepath.Join(dir, "untrusted")

	if err := os.MkdirAll(untrustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	err := store.UntrustSubtree(untrustedDir)
	if err == nil {
		t.Error("UntrustSubtree(not trusted) should return error")
	}
}

func TestListTrustedSubtrees_ReturnsAllPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	dir1 := filepath.Join(dir, "dir1")
	dir2 := filepath.Join(dir, "dir2")

	if err := os.MkdirAll(dir1, 0755); err != nil {
		t.Fatalf("mkdir dir1: %v", err)
	}
	if err := os.MkdirAll(dir2, 0755); err != nil {
		t.Fatalf("mkdir dir2: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Initially empty
	paths, err := store.ListTrustedSubtrees()
	if err != nil {
		t.Fatalf("ListTrustedSubtrees: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("initially, ListTrustedSubtrees() = %v, want empty", paths)
	}

	// Trust both directories
	if err := store.TrustSubtree(dir1); err != nil {
		t.Fatalf("TrustSubtree(dir1): %v", err)
	}
	if err := store.TrustSubtree(dir2); err != nil {
		t.Fatalf("TrustSubtree(dir2): %v", err)
	}

	// Should list both
	paths, err = store.ListTrustedSubtrees()
	if err != nil {
		t.Fatalf("ListTrustedSubtrees: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("ListTrustedSubtrees() returned %d paths, want 2", len(paths))
	}

	// Check both paths are present
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	if !pathSet[dir1] {
		t.Errorf("ListTrustedSubtrees() missing %s", dir1)
	}
	if !pathSet[dir2] {
		t.Errorf("ListTrustedSubtrees() missing %s", dir2)
	}
}

func TestIsTrustedSubtree_ChecksParentPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	nestedPath := filepath.Join(trustedDir, "a", "b", "c", ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Trust the parent
	if err := store.TrustSubtree(trustedDir); err != nil {
		t.Fatalf("TrustSubtree: %v", err)
	}

	// Deeply nested path should be trusted
	if !store.IsTrustedSubtree(nestedPath) {
		t.Error("IsTrustedSubtree(nested) = false, want true")
	}

	// Path outside trusted dir should not be trusted
	outsidePath := filepath.Join(dir, "other", ".envrc")
	if store.IsTrustedSubtree(outsidePath) {
		t.Error("IsTrustedSubtree(outside) = true, want false")
	}
}

func TestDeny_TakesPrecedenceOverTrust(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Trust the directory
	if err := store.TrustSubtree(trustedDir); err != nil {
		t.Fatalf("TrustSubtree: %v", err)
	}

	// Verify it's allowed via trust
	if status := store.Check(rc); status != Allowed {
		t.Fatalf("after trust, Check() = %v, want Allowed", status)
	}

	// Deny the specific file
	if err := store.Deny(rc); err != nil {
		t.Fatalf("Deny: %v", err)
	}

	// Deny should take precedence over trust
	status := store.Check(rc)
	if status != Denied {
		t.Errorf("after deny, Check() = %v, want Denied", status)
	}
}

func TestExplicitAllow_TakesPrecedenceOverTrust(t *testing.T) {
	t.Parallel()

	// This test verifies the priority order: explicit allow is checked before trust
	// Both should result in Allowed, but explicit allow is preferred

	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	trustedDir := filepath.Join(dir, "trusted")
	envrcPath := filepath.Join(trustedDir, ".envrc")

	if err := os.MkdirAll(trustedDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte("export FOO=bar"), 0644); err != nil {
		t.Fatalf("write envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	store := NewStoreWithBase(storeDir)

	// Trust the directory
	if err := store.TrustSubtree(trustedDir); err != nil {
		t.Fatalf("TrustSubtree: %v", err)
	}

	// Also explicitly allow the file
	if err := store.Allow(rc); err != nil {
		t.Fatalf("Allow: %v", err)
	}

	// Should still be allowed
	status := store.Check(rc)
	if status != Allowed {
		t.Errorf("Check() = %v, want Allowed", status)
	}

	// Now untrust - should still be allowed via explicit allow
	if err := store.UntrustSubtree(trustedDir); err != nil {
		t.Fatalf("UntrustSubtree: %v", err)
	}

	status = store.Check(rc)
	if status != Allowed {
		t.Errorf("after untrust, Check() = %v, want Allowed (via explicit allow)", status)
	}
}

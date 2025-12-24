package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
)

func TestCache_GetSet(t *testing.T) {
	// Use temp dir as cache location
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	key := "test-key-abc123"
	result := &Result{
		Env: env.Env{
			"FOO": "bar",
			"BAZ": "qux",
		},
	}

	// Initially should be a miss
	if got, ok := cache.Get(key); ok {
		t.Errorf("expected cache miss, got %v", got)
	}

	// Set the value
	if err := cache.Set(key, result, "/path/to/.envrc"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Now should be a hit
	got, ok := cache.Get(key)
	if !ok {
		t.Fatal("expected cache hit, got miss")
	}

	if got.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", got.Env["FOO"], "bar")
	}
	if got.Env["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", got.Env["BAZ"], "qux")
	}
}

func TestCache_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	// Add some entries
	for i := range 3 {
		key := "key-" + string(rune('a'+i))
		if err := cache.Set(key, &Result{Env: env.Env{"N": string(rune('0' + i))}}, "/test"); err != nil {
			t.Fatalf("Set: %v", err)
		}
	}

	// Verify they exist
	if _, ok := cache.Get("key-a"); !ok {
		t.Fatal("expected key-a to exist")
	}

	// Clear
	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	// Verify they're gone
	if _, ok := cache.Get("key-a"); ok {
		t.Error("expected key-a to be cleared")
	}
	if _, ok := cache.Get("key-b"); ok {
		t.Error("expected key-b to be cleared")
	}
}

func TestCacheKey_DifferentInputEnv(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte(`export FOO=bar`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	env1 := env.Env{"PATH": "/usr/bin"}
	env2 := env.Env{"PATH": "/usr/local/bin"}

	key1 := CacheKey(rc, env1)
	key2 := CacheKey(rc, env2)

	if key1 == key2 {
		t.Errorf("expected different keys for different input envs, got same: %s", key1)
	}
}

func TestCacheKey_DifferentFileContent(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")

	// Create first version
	if err := os.WriteFile(envrcPath, []byte(`export FOO=bar`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc1, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	inputEnv := env.Env{"PATH": "/usr/bin"}
	key1 := CacheKey(rc1, inputEnv)

	// Modify the file
	if err := os.WriteFile(envrcPath, []byte(`export FOO=baz`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc2, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	key2 := CacheKey(rc2, inputEnv)

	if key1 == key2 {
		t.Errorf("expected different keys for different file content, got same: %s", key1)
	}
}

func TestCacheKey_SameInputsSameKey(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte(`export FOO=bar`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	inputEnv := env.Env{"PATH": "/usr/bin", "HOME": "/home/test"}

	key1 := CacheKey(rc, inputEnv)
	key2 := CacheKey(rc, inputEnv)

	if key1 != key2 {
		t.Errorf("expected same keys for same inputs, got %s and %s", key1, key2)
	}
}

func TestEvaluator_CacheHit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create .envrc
	envrcPath := filepath.Join(tmpDir, "project", ".envrc")
	if err := os.MkdirAll(filepath.Dir(envrcPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte(`export FOO="bar"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	evaluator, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	evaluator = evaluator.WithCache(cache)

	inputEnv := env.Env{"HOME": "/home/test"}

	// First evaluation - should execute and cache
	result1, err := evaluator.Evaluate(rc, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate (first): %v", err)
	}

	if result1.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", result1.Env["FOO"], "bar")
	}

	// Verify it's in the cache
	key := CacheKey(rc, inputEnv)
	if _, ok := cache.Get(key); !ok {
		t.Error("expected result to be cached")
	}

	// Second evaluation - should hit cache
	result2, err := evaluator.Evaluate(rc, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate (second): %v", err)
	}

	if result2.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", result2.Env["FOO"], "bar")
	}
}

func TestEvaluator_CacheMissOnEnvChange(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create .envrc that uses a custom variable
	envrcPath := filepath.Join(tmpDir, "project", ".envrc")
	if err := os.MkdirAll(filepath.Dir(envrcPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(envrcPath, []byte(`export MYVAR="$INPUTVAR"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	evaluator, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	evaluator = evaluator.WithCache(cache)

	// First evaluation with INPUTVAR=first
	inputEnv1 := env.Env{"PATH": "/usr/bin:/bin", "INPUTVAR": "first"}
	result1, err := evaluator.Evaluate(rc, inputEnv1)
	if err != nil {
		t.Fatalf("Evaluate (first): %v", err)
	}

	if result1.Env["MYVAR"] != "first" {
		t.Errorf("MYVAR = %q, want %q", result1.Env["MYVAR"], "first")
	}

	// Second evaluation with different INPUTVAR - should NOT hit cache
	inputEnv2 := env.Env{"PATH": "/usr/bin:/bin", "INPUTVAR": "second"}
	result2, err := evaluator.Evaluate(rc, inputEnv2)
	if err != nil {
		t.Fatalf("Evaluate (second): %v", err)
	}

	if result2.Env["MYVAR"] != "second" {
		t.Errorf("MYVAR = %q, want %q", result2.Env["MYVAR"], "second")
	}
}

func TestEvaluator_CacheMissOnFileChange(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	envrcPath := filepath.Join(tmpDir, "project", ".envrc")
	if err := os.MkdirAll(filepath.Dir(envrcPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// First version
	if err := os.WriteFile(envrcPath, []byte(`export FOO="first"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc1, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	evaluator, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	evaluator = evaluator.WithCache(cache)

	inputEnv := env.Env{"HOME": "/home/test"}

	// First evaluation
	result1, err := evaluator.Evaluate(rc1, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate (first): %v", err)
	}

	if result1.Env["FOO"] != "first" {
		t.Errorf("FOO = %q, want %q", result1.Env["FOO"], "first")
	}

	// Modify the file
	if err := os.WriteFile(envrcPath, []byte(`export FOO="second"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	// Create new RC (which will have different content hash)
	rc2, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	// Second evaluation - should NOT hit cache due to file change
	result2, err := evaluator.Evaluate(rc2, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate (second): %v", err)
	}

	if result2.Env["FOO"] != "second" {
		t.Errorf("FOO = %q, want %q", result2.Env["FOO"], "second")
	}
}

func TestCache_CorruptedEntry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	// Write a corrupted cache file
	corruptedPath := filepath.Join(tmpDir, "cascade", "corrupted-key.json")
	if err := os.WriteFile(corruptedPath, []byte("not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupted file: %v", err)
	}

	// Should return miss, not error
	if _, ok := cache.Get("corrupted-key"); ok {
		t.Error("expected cache miss for corrupted entry")
	}
}

func TestNewCache_DefaultLocation(t *testing.T) {
	// Unset XDG_CACHE_HOME to test default behavior
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CACHE_HOME", "")

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	// Should have created ~/.cache/cascade
	expectedDir := filepath.Join(tmpHome, ".cache", "cascade")
	if cache.dir != expectedDir {
		t.Errorf("cache.dir = %q, want %q", cache.dir, expectedDir)
	}

	// Directory should exist
	if _, err := os.Stat(expectedDir); err != nil {
		t.Errorf("cache directory not created: %v", err)
	}
}

func TestEvaluator_WithoutCache(t *testing.T) {
	tmpDir := t.TempDir()

	envrcPath := filepath.Join(tmpDir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte(`export FOO="bar"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	// Create evaluator WITHOUT cache
	evaluator, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	inputEnv := env.Env{"HOME": "/home/test"}

	// Should work without cache
	result, err := evaluator.Evaluate(rc, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", result.Env["FOO"], "bar")
	}
}

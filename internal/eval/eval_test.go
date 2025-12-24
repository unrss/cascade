package eval

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
)

// testStdlib is a minimal stdlib for testing that outputs JSON to fd 3.
// It mimics the real stdlib's __main__ and __dump_at_exit behavior.
const testStdlib = `
set -euo pipefail

__main__() {
    local envrc_file="${1:-}"
    if [[ -z "$envrc_file" ]]; then
        echo "no .envrc file specified" >&2
        exit 1
    fi
    if [[ ! -f "$envrc_file" ]]; then
        echo "file not found: $envrc_file" >&2
        exit 1
    fi
    export CASCADE_DIR
    CASCADE_DIR="$(cd "$(dirname "$envrc_file")" && pwd)"
    trap __dump_at_exit EXIT
    source "$envrc_file"
}

__dump_at_exit() {
    local ret=$?
    trap - EXIT
    if [[ -n "${CASCADE_BIN:-}" ]]; then
        "$CASCADE_BIN" dump json >&3 2>/dev/null || true
    fi
    exit "$ret"
}
`

func TestEvaluate_SimpleExport(t *testing.T) {
	// Create a temp directory with a .envrc
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")
	if err := os.WriteFile(envrcPath, []byte(`export FOO="bar"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	// We need a mock cascade binary that implements "dump json"
	cascadeBin := createMockCascadeBin(t, tmpDir)

	eval, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	inputEnv := env.Env{
		"HOME": "/home/test",
		"PATH": "/usr/bin",
	}

	result, err := eval.Evaluate(rc, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Env["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", result.Env["FOO"], "bar")
	}

	// Original env should be preserved
	if result.Env["HOME"] != "/home/test" {
		t.Errorf("HOME = %q, want %q", result.Env["HOME"], "/home/test")
	}
}

func TestEvaluate_ModifyPATH(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a bin directory to add to PATH
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.Mkdir(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	envrcPath := filepath.Join(tmpDir, ".envrc")
	// Use PATH_add which is in the real stdlib, but for testing we'll just export directly
	envrcContent := `export PATH="` + binDir + `:$PATH"`
	if err := os.WriteFile(envrcPath, []byte(envrcContent), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	eval, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	inputEnv := env.Env{
		"PATH": "/usr/bin:/bin",
	}

	result, err := eval.Evaluate(rc, inputEnv)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !strings.HasPrefix(result.Env["PATH"], binDir+":") {
		t.Errorf("PATH = %q, want prefix %q", result.Env["PATH"], binDir+":")
	}
}

func TestEvaluate_SyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")
	// Invalid bash syntax
	if err := os.WriteFile(envrcPath, []byte(`export FOO="`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	eval, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = eval.Evaluate(rc, env.Env{})
	if err == nil {
		t.Fatal("expected error for syntax error, got nil")
	}

	if !strings.Contains(err.Error(), "exited with status") {
		t.Errorf("error = %q, want to contain 'exited with status'", err.Error())
	}
}

func TestEvaluate_CascadeDirSet(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")
	// Export CASCADE_DIR so we can verify it was set correctly
	if err := os.WriteFile(envrcPath, []byte(`export TEST_DIR="$CASCADE_DIR"`), 0o644); err != nil {
		t.Fatalf("write .envrc: %v", err)
	}

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	eval, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	result, err := eval.Evaluate(rc, env.Env{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// CASCADE_DIR should be the directory containing the .envrc
	if result.Env["TEST_DIR"] != tmpDir {
		t.Errorf("TEST_DIR = %q, want %q", result.Env["TEST_DIR"], tmpDir)
	}
}

func TestEvaluate_NonExistentRC(t *testing.T) {
	tmpDir := t.TempDir()
	envrcPath := filepath.Join(tmpDir, ".envrc")

	rc, err := envrc.NewRC(envrcPath)
	if err != nil {
		t.Fatalf("NewRC: %v", err)
	}

	if rc.Exists {
		t.Fatal("expected rc.Exists to be false")
	}

	cascadeBin := createMockCascadeBin(t, tmpDir)

	eval, err := New("", testStdlib, cascadeBin)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = eval.Evaluate(rc, env.Env{})
	if err == nil {
		t.Fatal("expected error for non-existent rc, got nil")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error = %q, want to contain 'does not exist'", err.Error())
	}
}

func TestDumpJSON(t *testing.T) {
	e := env.Env{
		"FOO": "bar",
		"BAZ": "qux",
	}

	var buf bytes.Buffer
	if err := DumpJSON(e, &buf); err != nil {
		t.Fatalf("DumpJSON: %v", err)
	}

	// Parse it back
	parsed, err := ParseJSON(&buf)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	if parsed["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", parsed["FOO"], "bar")
	}
	if parsed["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", parsed["BAZ"], "qux")
	}
}

func TestParseJSON_Invalid(t *testing.T) {
	_, err := ParseJSON(strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestNew_Validation(t *testing.T) {
	tests := []struct {
		name     string
		stdlib   string
		selfPath string
		wantErr  string
	}{
		{
			name:     "empty stdlib",
			stdlib:   "",
			selfPath: "/usr/bin/cascade",
			wantErr:  "stdlib content is required",
		},
		{
			name:     "empty selfPath",
			stdlib:   "some stdlib",
			selfPath: "",
			wantErr:  "selfPath is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New("", tt.stdlib, tt.selfPath)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDumpJSON_SpecialCharacters(t *testing.T) {
	e := env.Env{
		"NORMAL":    "value",
		"QUOTES":    `value with "quotes"`,
		"NEWLINE":   "line1\nline2",
		"TAB":       "col1\tcol2",
		"BACKSLASH": `path\to\file`,
	}

	var buf bytes.Buffer
	if err := DumpJSON(e, &buf); err != nil {
		t.Fatalf("DumpJSON: %v", err)
	}

	// Parse it back and verify
	parsed, err := ParseJSON(&buf)
	if err != nil {
		t.Fatalf("ParseJSON: %v", err)
	}

	for key, want := range e {
		if got := parsed[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
}

// createMockCascadeBin creates a shell script that mimics "cascade dump json".
// It outputs the current environment as JSON.
func createMockCascadeBin(t *testing.T, dir string) string {
	t.Helper()

	binPath := filepath.Join(dir, "cascade-mock")

	// This script outputs env as JSON when called with "dump json"
	script := `#!/bin/bash
if [[ "$1" == "dump" && "$2" == "json" ]]; then
    echo -n "{"
    first=true
    while IFS='=' read -r -d '' key value; do
        if [[ -n "$key" ]]; then
            if [[ "$first" == "true" ]]; then
                first=false
            else
                echo -n ","
            fi
            # Escape special characters in value
            value="${value//\\/\\\\}"
            value="${value//\"/\\\"}"
            value="${value//$'\n'/\\n}"
            value="${value//$'\t'/\\t}"
            value="${value//$'\r'/\\r}"
            echo -n "\"$key\":\"$value\""
        fi
    done < <(env -0)
    echo "}"
    exit 0
fi
exit 1
`

	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write mock cascade: %v", err)
	}

	return binPath
}

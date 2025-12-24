package cmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// testBinary holds the path to the compiled cascade binary.
// Built once via TestMain, reused across all integration tests.
var (
	testBinary     string
	testBinaryOnce sync.Once
	testBinaryErr  error
)

// buildTestBinary compiles the cascade binary for testing.
// Returns the path to the binary or an error.
func buildTestBinary(t *testing.T) string {
	t.Helper()

	testBinaryOnce.Do(func() {
		// Create a temp directory for the binary.
		// Note: Can't use t.TempDir() here because this runs in sync.Once
		// and the directory must persist across all tests.
		tmpDir, err := os.MkdirTemp("", "cascade-test-*") //nolint:usetesting // sync.Once requires persistent dir
		if err != nil {
			testBinaryErr = err
			return
		}

		testBinary = filepath.Join(tmpDir, "cascade")

		// Build the binary using the module path
		cmd := exec.Command("go", "build", "-o", testBinary, "github.com/unrss/cascade/cmd/cascade")
		output, err := cmd.CombinedOutput()
		if err != nil {
			testBinaryErr = &buildError{output: output, err: err}
			return
		}
	})

	if testBinaryErr != nil {
		t.Fatalf("build cascade binary: %v", testBinaryErr)
	}

	return testBinary
}

type buildError struct {
	output []byte
	err    error
}

func (e *buildError) Error() string {
	return string(e.output) + ": " + e.err.Error()
}

// testEnv holds the test environment configuration.
type testEnv struct {
	t         *testing.T
	binary    string
	homeDir   string
	dataDir   string
	workDir   string
	baseEnv   []string
	stderrBuf *bytes.Buffer
}

// setupTestEnv creates an isolated test environment.
// homeDir is used as HOME and the root for .envrc chain discovery.
// dataDir is used as XDG_DATA_HOME for allow/deny state.
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	binary := buildTestBinary(t)
	tmpDir := t.TempDir()

	// Resolve symlinks to avoid macOS /var -> /private/var issues.
	// The cascade binary resolves symlinks internally, so we need to match.
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("eval symlinks: %v", err)
	}

	homeDir := filepath.Join(tmpDir, "home")
	dataDir := filepath.Join(tmpDir, "data")

	if err := os.MkdirAll(homeDir, 0755); err != nil {
		t.Fatalf("mkdir home: %v", err)
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}

	// Build minimal base environment
	baseEnv := []string{
		"HOME=" + homeDir,
		"XDG_DATA_HOME=" + dataDir,
		"PATH=" + os.Getenv("PATH"),
		"SHELL=/bin/bash",
	}

	return &testEnv{
		t:         t,
		binary:    binary,
		homeDir:   homeDir,
		dataDir:   dataDir,
		workDir:   homeDir,
		baseEnv:   baseEnv,
		stderrBuf: new(bytes.Buffer),
	}
}

// withWorkDir returns a copy of testEnv with a different working directory.
func (e *testEnv) withWorkDir(dir string) *testEnv {
	cp := *e
	cp.workDir = dir
	cp.stderrBuf = new(bytes.Buffer)
	return &cp
}

// withEnv returns a copy of testEnv with additional environment variables.
func (e *testEnv) withEnv(extra ...string) *testEnv {
	cp := *e
	cp.baseEnv = append(append([]string{}, e.baseEnv...), extra...)
	cp.stderrBuf = new(bytes.Buffer)
	return &cp
}

// run executes cascade with the given arguments.
// Returns stdout, stderr, and any error.
func (e *testEnv) run(args ...string) (stdout, stderr string, err error) {
	e.t.Helper()

	cmd := exec.Command(e.binary, args...) //nolint:gosec // intentional CLI test harness
	cmd.Dir = e.workDir
	cmd.Env = e.baseEnv

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}

// runExport runs "cascade export bash" and returns the output.
func (e *testEnv) runExport() (stdout, stderr string, err error) {
	return e.run("export", "bash")
}

// runAllow runs "cascade allow" on the given path (or current dir if empty).
func (e *testEnv) runAllow(path string) error {
	e.t.Helper()
	args := []string{"allow"}
	if path != "" {
		args = append(args, path)
	}
	_, stderr, err := e.run(args...)
	if err != nil {
		e.t.Logf("allow stderr: %s", stderr)
	}
	return err
}

// runDeny runs "cascade deny" on the given path (or current dir if empty).
func (e *testEnv) runDeny(path string) error {
	e.t.Helper()
	args := []string{"deny"}
	if path != "" {
		args = append(args, path)
	}
	_, stderr, err := e.run(args...)
	if err != nil {
		e.t.Logf("deny stderr: %s", stderr)
	}
	return err
}

// runHook runs "cascade hook bash" and returns the output.
func (e *testEnv) runHook() (string, error) {
	stdout, _, err := e.run("hook", "bash")
	return stdout, err
}

// runStatus runs "cascade status" and returns the output.
func (e *testEnv) runStatus() (stdout, stderr string, err error) {
	return e.run("status")
}

// createEnvrc creates a .envrc file in the given directory.
func (e *testEnv) createEnvrc(dir, content string) {
	e.t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, ".envrc")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		e.t.Fatalf("write %s: %v", path, err)
	}
}

// createDir creates a directory (for bin directories, etc.).
func (e *testEnv) createDir(dir string) {
	e.t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		e.t.Fatalf("mkdir %s: %v", dir, err)
	}
}

// parseExport parses bash export output into a map.
// Handles: export KEY="value"; and unset KEY;
func parseExport(output string) map[string]string {
	result := make(map[string]string)

	// Match export KEY="value";
	exportRe := regexp.MustCompile(`export ([A-Za-z_][A-Za-z0-9_]*)="([^"]*)";`)
	for _, match := range exportRe.FindAllStringSubmatch(output, -1) {
		key := match[1]
		value := match[2]
		// Unescape common bash escapes
		value = strings.ReplaceAll(value, `\"`, `"`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		result[key] = value
	}

	// Match unset KEY;
	unsetRe := regexp.MustCompile(`unset ([A-Za-z_][A-Za-z0-9_]*);`)
	for _, match := range unsetRe.FindAllStringSubmatch(output, -1) {
		result[match[1]] = "" // Empty string indicates unset
	}

	return result
}

// assertExportContains checks that the export output sets the expected key=value.
func assertExportContains(t *testing.T, exports map[string]string, key, wantValue string) {
	t.Helper()
	got, ok := exports[key]
	if !ok {
		t.Errorf("export missing key %q, want value %q", key, wantValue)
		return
	}
	if got != wantValue {
		t.Errorf("export %s = %q, want %q", key, got, wantValue)
	}
}

// assertExportNotContains checks that the export output does not set the key.
func assertExportNotContains(t *testing.T, exports map[string]string, key string) {
	t.Helper()
	if got, ok := exports[key]; ok {
		t.Errorf("export unexpectedly contains %s = %q", key, got)
	}
}

// assertStderrContains checks that stderr contains the expected substring.
func assertStderrContains(t *testing.T, stderr, want string) {
	t.Helper()
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want to contain %q", stderr, want)
	}
}

// assertStderrNotContains checks that stderr does not contain the substring.
func assertStderrNotContains(t *testing.T, stderr, notWant string) {
	t.Helper()
	if strings.Contains(stderr, notWant) {
		t.Errorf("stderr = %q, should not contain %q", stderr, notWant)
	}
}

// TestIntegration_BasicInheritance tests that child .envrc values override parent values.
func TestIntegration_BasicInheritance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create directory structure: home/.envrc and home/work/.envrc
	workDir := filepath.Join(env.homeDir, "work")
	env.createEnvrc(env.homeDir, `export HOME_VAR="from_home"`)
	env.createEnvrc(workDir, `export WORK_VAR="from_work"
export HOME_VAR="overridden"`)

	// Allow both files
	if err := env.runAllow(filepath.Join(env.homeDir, ".envrc")); err != nil {
		t.Fatalf("allow home: %v", err)
	}
	if err := env.runAllow(filepath.Join(workDir, ".envrc")); err != nil {
		t.Fatalf("allow work: %v", err)
	}

	// Run export from work directory
	workEnv := env.withWorkDir(workDir)
	stdout, stderr, err := workEnv.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)

	// Child should override parent
	assertExportContains(t, exports, "HOME_VAR", "overridden")
	assertExportContains(t, exports, "WORK_VAR", "from_work")

	// CASCADE_DIR should be set to the deepest directory
	assertExportContains(t, exports, "CASCADE_DIR", workDir)
}

// TestIntegration_AllowDenyFlow tests the allow/deny workflow.
func TestIntegration_AllowDenyFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create .envrc
	env.createEnvrc(env.homeDir, `export TEST_VAR="test_value"`)

	// Before allow: export should show "not allowed" warning
	stdout, stderr, _ := env.runExport()
	assertStderrContains(t, stderr, "not allowed")

	// Export output should be empty (no exports)
	exports := parseExport(stdout)
	assertExportNotContains(t, exports, "TEST_VAR")

	// Allow the file
	if err := env.runAllow(""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// After allow: export should work
	stdout, stderr, err := env.runExport()
	if err != nil {
		t.Fatalf("export after allow: %v\nstderr: %s", err, stderr)
	}

	exports = parseExport(stdout)
	assertExportContains(t, exports, "TEST_VAR", "test_value")
	assertStderrNotContains(t, stderr, "not allowed")
}

// TestIntegration_DeniedFile tests that denied files show error and don't apply.
func TestIntegration_DeniedFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create and allow .envrc
	env.createEnvrc(env.homeDir, `export TEST_VAR="test_value"`)
	if err := env.runAllow(""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Verify it works
	stdout, _, err := env.runExport()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	exports := parseExport(stdout)
	assertExportContains(t, exports, "TEST_VAR", "test_value")

	// Now deny the file
	if err := env.runDeny(""); err != nil {
		t.Fatalf("deny: %v", err)
	}

	// Export should show error and not apply
	stdout, stderr, _ := env.runExport()
	assertStderrContains(t, stderr, "blocked")

	exports = parseExport(stdout)
	assertExportNotContains(t, exports, "TEST_VAR")
}

// TestIntegration_SourceEnvSibling tests source_env with sibling directories.
func TestIntegration_SourceEnvSibling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create sibling directory structure
	sharedDir := filepath.Join(env.homeDir, "shared")
	projectDir := filepath.Join(env.homeDir, "project")

	env.createEnvrc(sharedDir, `export SHARED="shared_value"`)
	env.createEnvrc(projectDir, `source_env ../shared
export PROJECT="project_value"`)

	// Allow both files
	if err := env.runAllow(filepath.Join(sharedDir, ".envrc")); err != nil {
		t.Fatalf("allow shared: %v", err)
	}
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow project: %v", err)
	}

	// Run export from project directory
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, err := projectEnv.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)

	// Both shared and project vars should be set
	assertExportContains(t, exports, "SHARED", "shared_value")
	assertExportContains(t, exports, "PROJECT", "project_value")
}

// TestIntegration_SourceEnv_NotAllowed tests that source_env refuses to source
// a sibling .envrc that is not allowed.
func TestIntegration_SourceEnv_NotAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create sibling directory structure
	sharedDir := filepath.Join(env.homeDir, "shared")
	projectDir := filepath.Join(env.homeDir, "project")

	env.createEnvrc(sharedDir, `export SHARED="shared_value"`)
	env.createEnvrc(projectDir, `source_env ../shared
export PROJECT="project_value"`)

	// Allow ONLY project, NOT shared
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow project: %v", err)
	}
	// Note: shared is NOT allowed

	// Run export from project directory
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, _ := projectEnv.runExport()

	exports := parseExport(stdout)

	// SHARED should NOT be present (not allowed)
	assertExportNotContains(t, exports, "SHARED")

	// Should see error about shared not being allowed
	assertStderrContains(t, stderr, "not allowed")
}

// TestIntegration_SourceEnv_Denied tests that source_env refuses to source
// a sibling .envrc that is explicitly denied.
func TestIntegration_SourceEnv_Denied(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create sibling directory structure
	sharedDir := filepath.Join(env.homeDir, "shared")
	projectDir := filepath.Join(env.homeDir, "project")

	env.createEnvrc(sharedDir, `export SHARED="shared_value"`)
	env.createEnvrc(projectDir, `source_env ../shared
export PROJECT="project_value"`)

	// Allow project, DENY shared
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow project: %v", err)
	}
	if err := env.runDeny(filepath.Join(sharedDir, ".envrc")); err != nil {
		t.Fatalf("deny shared: %v", err)
	}

	// Run export from project directory
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, _ := projectEnv.runExport()

	exports := parseExport(stdout)

	// SHARED should NOT be present (denied)
	assertExportNotContains(t, exports, "SHARED")

	// Should see error about shared not being allowed
	assertStderrContains(t, stderr, "not allowed")
}

// TestIntegration_SourceEnvIfExists_NotAllowed tests that source_env_if_exists
// also respects allow/deny settings.
func TestIntegration_SourceEnvIfExists_NotAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create directory structure with a private env file
	projectDir := filepath.Join(env.homeDir, "project")
	env.createDir(projectDir)

	// Create a .envrc.local file (like a private override)
	localEnvPath := filepath.Join(projectDir, ".envrc.local")
	if err := os.WriteFile(localEnvPath, []byte(`export LOCAL_SECRET="secret_value"`), 0644); err != nil {
		t.Fatalf("write .envrc.local: %v", err)
	}

	// Create main .envrc that sources the local file
	env.createEnvrc(projectDir, `source_env_if_exists .envrc.local
export PROJECT="project_value"`)

	// Allow ONLY the main .envrc, NOT the .envrc.local
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow project: %v", err)
	}
	// Note: .envrc.local is NOT allowed

	// Run export from project directory
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, _ := projectEnv.runExport()

	exports := parseExport(stdout)

	// LOCAL_SECRET should NOT be present (not allowed)
	assertExportNotContains(t, exports, "LOCAL_SECRET")

	// Should see error about .envrc.local not being allowed
	assertStderrContains(t, stderr, "not allowed")
}

// TestIntegration_PathAdd tests PATH_add functionality.
func TestIntegration_PathAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create project with bin directory
	projectDir := filepath.Join(env.homeDir, "project")
	binDir := filepath.Join(projectDir, "bin")
	env.createDir(binDir)
	env.createEnvrc(projectDir, `PATH_add ./bin
export MY_VAR="test"`)

	// Allow the file
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Run export
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, err := projectEnv.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)

	// MY_VAR should be set
	assertExportContains(t, exports, "MY_VAR", "test")

	// PATH should contain the bin directory
	pathValue, ok := exports["PATH"]
	if !ok {
		t.Fatal("PATH not in exports")
	}
	if !strings.Contains(pathValue, binDir) {
		t.Errorf("PATH = %q, want to contain %q", pathValue, binDir)
	}
}

// TestIntegration_CdOut tests environment reversion when leaving a directory.
func TestIntegration_CdOut(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create .envrc in a subdirectory
	projectDir := filepath.Join(env.homeDir, "project")
	env.createEnvrc(projectDir, `export PROJECT_VAR="in_project"`)

	// Allow the file
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Run export from project directory to get CASCADE_DIFF
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, err := projectEnv.runExport()
	if err != nil {
		t.Fatalf("export in project: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)
	cascadeDiff, ok := exports["CASCADE_DIFF"]
	if !ok {
		t.Fatal("CASCADE_DIFF not set after export")
	}

	// Now simulate moving to a directory without .envrc
	// by running export with CASCADE_DIFF set but from home (no .envrc there)
	homeEnv := env.withEnv("CASCADE_DIFF=" + cascadeDiff)
	stdout, _, err = homeEnv.runExport()
	if err != nil {
		t.Fatalf("export in home: %v", err)
	}

	exports = parseExport(stdout)

	// CASCADE_DIFF should be unset (reverted)
	if _, ok := exports["CASCADE_DIFF"]; ok {
		// Check if it's an unset command
		if !strings.Contains(stdout, "unset CASCADE_DIFF") {
			t.Error("CASCADE_DIFF should be unset when leaving envrc directory")
		}
	}

	// CASCADE_DIR should be unset
	if !strings.Contains(stdout, "unset CASCADE_DIR") {
		t.Error("CASCADE_DIR should be unset when leaving envrc directory")
	}
}

// TestIntegration_HookOutput tests that hook output contains expected shell setup.
func TestIntegration_HookOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	stdout, err := env.runHook()
	if err != nil {
		t.Fatalf("hook: %v", err)
	}

	// Should contain _cascade_hook function
	if !strings.Contains(stdout, "_cascade_hook()") {
		t.Error("hook output missing _cascade_hook function")
	}

	// Should contain PROMPT_COMMAND setup
	if !strings.Contains(stdout, "PROMPT_COMMAND") {
		t.Error("hook output missing PROMPT_COMMAND setup")
	}

	// Should contain the cascade binary path
	if !strings.Contains(stdout, env.binary) {
		t.Errorf("hook output missing binary path %q", env.binary)
	}
}

// TestIntegration_Status tests the status command output.
func TestIntegration_Status(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create .envrc chain
	workDir := filepath.Join(env.homeDir, "work")
	env.createEnvrc(env.homeDir, `export HOME_VAR="from_home"`)
	env.createEnvrc(workDir, `export WORK_VAR="from_work"`)

	// Allow home, leave work not allowed
	if err := env.runAllow(filepath.Join(env.homeDir, ".envrc")); err != nil {
		t.Fatalf("allow home: %v", err)
	}

	// Run status from work directory
	workEnv := env.withWorkDir(workDir)
	stdout, _, err := workEnv.runStatus()
	if err != nil {
		t.Fatalf("status: %v", err)
	}

	// Should show both files
	if !strings.Contains(stdout, ".envrc") {
		t.Error("status output missing .envrc files")
	}

	// Should show allowed status for home
	if !strings.Contains(stdout, "allowed") {
		t.Error("status output missing 'allowed' status")
	}

	// Should show not allowed status for work
	if !strings.Contains(stdout, "not allowed") {
		t.Error("status output missing 'not allowed' status")
	}
}

// TestIntegration_DeepInheritance tests 4-level deep inheritance.
func TestIntegration_DeepInheritance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create 4-level deep structure
	aDir := filepath.Join(env.homeDir, "a")
	bDir := filepath.Join(aDir, "b")
	cDir := filepath.Join(bDir, "c")
	dDir := filepath.Join(cDir, "d")

	env.createEnvrc(aDir, `export LEVEL_A="from_a"`)
	env.createEnvrc(bDir, `export LEVEL_B="from_b"`)
	env.createEnvrc(cDir, `export LEVEL_C="from_c"`)
	env.createEnvrc(dDir, `export LEVEL_D="from_d"
export LEVEL_A="overridden_in_d"`)

	// Allow all files
	for _, dir := range []string{aDir, bDir, cDir, dDir} {
		if err := env.runAllow(filepath.Join(dir, ".envrc")); err != nil {
			t.Fatalf("allow %s: %v", dir, err)
		}
	}

	// Run export from deepest directory
	dEnv := env.withWorkDir(dDir)
	stdout, stderr, err := dEnv.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)

	// All levels should be present
	assertExportContains(t, exports, "LEVEL_A", "overridden_in_d") // Overridden
	assertExportContains(t, exports, "LEVEL_B", "from_b")
	assertExportContains(t, exports, "LEVEL_C", "from_c")
	assertExportContains(t, exports, "LEVEL_D", "from_d")
}

// TestIntegration_ContentChangeInvalidatesAllow tests that modifying .envrc requires re-allow.
func TestIntegration_ContentChangeInvalidatesAllow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create and allow .envrc
	env.createEnvrc(env.homeDir, `export TEST_VAR="original"`)
	if err := env.runAllow(""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Verify it works
	stdout, _, err := env.runExport()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	exports := parseExport(stdout)
	assertExportContains(t, exports, "TEST_VAR", "original")

	// Modify the file
	env.createEnvrc(env.homeDir, `export TEST_VAR="modified"`)

	// Export should now show "not allowed"
	stdout, stderr, _ := env.runExport()
	assertStderrContains(t, stderr, "not allowed")

	exports = parseExport(stdout)
	assertExportNotContains(t, exports, "TEST_VAR")

	// Re-allow
	if err := env.runAllow(""); err != nil {
		t.Fatalf("re-allow: %v", err)
	}

	// Now it should work with new value
	stdout, _, err = env.runExport()
	if err != nil {
		t.Fatalf("export after re-allow: %v", err)
	}
	exports = parseExport(stdout)
	assertExportContains(t, exports, "TEST_VAR", "modified")
}

// TestIntegration_PartialChainAllowed tests that only allowed files in chain are evaluated.
func TestIntegration_PartialChainAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create chain: home -> work -> project
	workDir := filepath.Join(env.homeDir, "work")
	projectDir := filepath.Join(workDir, "project")

	env.createEnvrc(env.homeDir, `export HOME_VAR="from_home"`)
	env.createEnvrc(workDir, `export WORK_VAR="from_work"`)
	env.createEnvrc(projectDir, `export PROJECT_VAR="from_project"`)

	// Only allow home and project, not work
	if err := env.runAllow(filepath.Join(env.homeDir, ".envrc")); err != nil {
		t.Fatalf("allow home: %v", err)
	}
	if err := env.runAllow(filepath.Join(projectDir, ".envrc")); err != nil {
		t.Fatalf("allow project: %v", err)
	}

	// Run export from project
	projectEnv := env.withWorkDir(projectDir)
	stdout, stderr, err := projectEnv.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	// Should warn about work not being allowed
	assertStderrContains(t, stderr, "not allowed")

	exports := parseExport(stdout)

	// Home and project should be set
	assertExportContains(t, exports, "HOME_VAR", "from_home")
	assertExportContains(t, exports, "PROJECT_VAR", "from_project")

	// Work should NOT be set (not allowed)
	assertExportNotContains(t, exports, "WORK_VAR")
}

// TestIntegration_UnsupportedShell tests error handling for unsupported shells.
func TestIntegration_UnsupportedShell(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	_, stderr, err := env.run("export", "powershell")
	if err == nil {
		t.Fatal("expected error for unsupported shell")
	}

	if !strings.Contains(stderr, "unsupported shell") {
		t.Errorf("stderr = %q, want to contain 'unsupported shell'", stderr)
	}
}

// TestIntegration_AllowNonExistent tests error handling for allowing non-existent files.
func TestIntegration_AllowNonExistent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Try to allow non-existent file
	_, stderr, err := env.run("allow", "/nonexistent/.envrc")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}

	if !strings.Contains(stderr, "does not exist") && !strings.Contains(stderr, "no such file") {
		t.Errorf("stderr = %q, want to contain error about non-existent file", stderr)
	}
}

// TestIntegration_EmptyEnvrc tests handling of empty .envrc files.
func TestIntegration_EmptyEnvrc(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create empty .envrc
	env.createEnvrc(env.homeDir, "")

	// Allow it
	if err := env.runAllow(""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Export should work without error
	stdout, stderr, err := env.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	// Should set CASCADE_DIR even for empty file
	exports := parseExport(stdout)
	assertExportContains(t, exports, "CASCADE_DIR", env.homeDir)
}

// TestIntegration_SpecialCharactersInValue tests handling of special characters.
func TestIntegration_SpecialCharactersInValue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create .envrc with special characters
	env.createEnvrc(env.homeDir, `export SPECIAL='value with spaces'
export QUOTED="double \"quotes\""`)

	// Allow it
	if err := env.runAllow(""); err != nil {
		t.Fatalf("allow: %v", err)
	}

	// Export should work
	stdout, stderr, err := env.runExport()
	if err != nil {
		t.Fatalf("export: %v\nstderr: %s", err, stderr)
	}

	exports := parseExport(stdout)
	assertExportContains(t, exports, "SPECIAL", "value with spaces")
}

// TestIntegration_VersionCommand tests the version command.
func TestIntegration_VersionCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	stdout, _, err := env.run("version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}

	// Should output something (version string)
	if len(strings.TrimSpace(stdout)) == 0 {
		t.Error("version output is empty")
	}
}

// TestIntegration_CheckCommand tests the check command for all statuses.
func TestIntegration_CheckCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := setupTestEnv(t)

	// Create .envrc
	envrcPath := filepath.Join(env.homeDir, ".envrc")
	env.createEnvrc(env.homeDir, `export TEST_VAR="test_value"`)

	// Test 1: Check before allow - should be "not allowed" with exit code 1
	stdout, _, err := env.run("check", envrcPath)
	if err == nil {
		t.Error("check should fail for not-allowed file")
	}
	if !strings.Contains(stdout, "not allowed") {
		t.Errorf("stdout = %q, want to contain 'not allowed'", stdout)
	}

	// Test 2: Check with --silent flag - should have no output
	stdout, _, err = env.run("check", "--silent", envrcPath)
	if err == nil {
		t.Error("check --silent should fail for not-allowed file")
	}
	if stdout != "" {
		t.Errorf("check --silent stdout = %q, want empty", stdout)
	}

	// Test 3: Allow the file, then check - should be "allowed" with exit code 0
	if err := env.runAllow(envrcPath); err != nil {
		t.Fatalf("allow: %v", err)
	}

	stdout, _, err = env.run("check", envrcPath)
	if err != nil {
		t.Errorf("check should succeed for allowed file: %v", err)
	}
	if !strings.Contains(stdout, "allowed") {
		t.Errorf("stdout = %q, want to contain 'allowed'", stdout)
	}

	// Test 4: Check with --silent flag for allowed file - should have no output, exit 0
	stdout, _, err = env.run("check", "--silent", envrcPath)
	if err != nil {
		t.Errorf("check --silent should succeed for allowed file: %v", err)
	}
	if stdout != "" {
		t.Errorf("check --silent stdout = %q, want empty", stdout)
	}

	// Test 5: Deny the file, then check - should be "denied" with exit code 1
	if err := env.runDeny(envrcPath); err != nil {
		t.Fatalf("deny: %v", err)
	}

	stdout, _, err = env.run("check", envrcPath)
	if err == nil {
		t.Error("check should fail for denied file")
	}
	if !strings.Contains(stdout, "denied") {
		t.Errorf("stdout = %q, want to contain 'denied'", stdout)
	}
}

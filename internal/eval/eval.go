package eval

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
)

// Result holds the output of an .envrc evaluation.
type Result struct {
	Env          env.Env  // Resulting environment variables
	ExtraWatches []string // Additional files to watch (from watch_file)
}

// Evaluator executes .envrc files and captures environment changes.
type Evaluator struct {
	bashPath string // Path to bash binary
	stdlib   string // Embedded stdlib.sh content
	selfPath string // Path to cascade binary (for callbacks)
	cache    *Cache // Optional cache for evaluation results
}

// New creates an Evaluator.
//
// bashPath: path to bash (if empty, uses exec.LookPath("bash"))
// stdlib: content of stdlib.sh (embedded via //go:embed)
// selfPath: path to cascade binary (for source_env callbacks)
func New(bashPath, stdlib, selfPath string) (*Evaluator, error) {
	if bashPath == "" {
		var err error
		bashPath, err = exec.LookPath("bash")
		if err != nil {
			return nil, fmt.Errorf("find bash: %w", err)
		}
	}

	if stdlib == "" {
		return nil, errors.New("stdlib content is required")
	}

	if selfPath == "" {
		return nil, errors.New("selfPath is required")
	}

	return &Evaluator{
		bashPath: bashPath,
		stdlib:   stdlib,
		selfPath: selfPath,
	}, nil
}

// WithCache returns a copy of the Evaluator with caching enabled.
func (e *Evaluator) WithCache(c *Cache) *Evaluator {
	cp := *e
	cp.cache = c
	return &cp
}

// Evaluate executes an RC file with the given input environment.
// Returns the resulting environment and any extra watched files.
//
// If caching is enabled, checks the cache first and stores results after evaluation.
//
// Process:
//  1. Check cache (if enabled)
//  2. Spawn bash with stdlib eval and __main__ call
//  3. Set CASCADE_BIN, CASCADE_DIR, CASCADE_STDLIB in subprocess env
//  4. Capture JSON from fd 3, let stderr pass through
//  5. Parse JSON to Env map
//  6. Extract CASCADE_EXTRA_WATCHES for additional file watching
//  7. Store result in cache (if enabled)
func (e *Evaluator) Evaluate(rc *envrc.RC, inputEnv env.Env) (*Result, error) {
	if !rc.Exists {
		return nil, fmt.Errorf("rc file does not exist: %s", rc.Path)
	}

	// Check cache first
	var cacheKey string
	if e.cache != nil {
		cacheKey = CacheKey(rc, inputEnv)
		if cached, ok := e.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}

	// Create pipe for fd 3 (JSON output)
	jsonReader, jsonWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create pipe: %w", err)
	}
	defer jsonReader.Close()

	// Build bash command: eval stdlib then call __main__
	script := fmt.Sprintf(`eval "$CASCADE_STDLIB" && __main__ %q`, rc.Path)

	cmd := exec.Command(e.bashPath, "-c", script) //nolint:gosec // intentional shell evaluation

	// Set up environment
	cmd.Env = inputEnv.ToGoEnv()
	cmd.Env = append(cmd.Env, "CASCADE_BIN="+e.selfPath)
	cmd.Env = append(cmd.Env, "CASCADE_DIR="+rc.Dir)
	cmd.Env = append(cmd.Env, "CASCADE_STDLIB="+e.stdlib)

	// fd 3 is the JSON output channel
	// ExtraFiles[0] becomes fd 3 in the child process
	cmd.ExtraFiles = []*os.File{jsonWriter}

	// Capture stdout for error messages, let stderr pass through
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		jsonWriter.Close()
		return nil, fmt.Errorf("start bash: %w", err)
	}

	// Close writer in parent so reader gets EOF when child exits
	jsonWriter.Close()

	// Read JSON output from fd 3
	var jsonBuf bytes.Buffer
	if _, err := io.Copy(&jsonBuf, jsonReader); err != nil {
		// Kill the process if we can't read
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("read json output: %w", err)
	}

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Include stdout in error message for debugging
			if stdout.Len() > 0 {
				return nil, fmt.Errorf("bash exited with status %d: %s", exitErr.ExitCode(), stdout.String())
			}
			return nil, fmt.Errorf("bash exited with status %d", exitErr.ExitCode())
		}
		return nil, fmt.Errorf("wait bash: %w", err)
	}

	// Parse JSON output
	if jsonBuf.Len() == 0 {
		return nil, errors.New("no json output from bash")
	}

	envResult, err := ParseJSON(&jsonBuf)
	if err != nil {
		return nil, fmt.Errorf("parse env output: %w", err)
	}

	// Extract extra watches from CASCADE_EXTRA_WATCHES
	var extraWatches []string
	if watches, ok := envResult["CASCADE_EXTRA_WATCHES"]; ok {
		for _, path := range strings.Split(watches, "\n") {
			path = strings.TrimSpace(path)
			if path != "" {
				extraWatches = append(extraWatches, path)
			}
		}
		delete(envResult, "CASCADE_EXTRA_WATCHES") // Don't export this internal variable
	}

	result := &Result{
		Env:          envResult,
		ExtraWatches: extraWatches,
	}

	// Store in cache
	if e.cache != nil && cacheKey != "" {
		// Ignore cache write errors - they're not fatal
		_ = e.cache.Set(cacheKey, result, rc.Path)
	}

	return result, nil
}

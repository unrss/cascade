package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
	"github.com/unrss/cascade/internal/eval"
	"github.com/unrss/cascade/internal/shell"
	"github.com/unrss/cascade/internal/state"
)

func newExportCmd(stdlib string) *cobra.Command {
	var noCache bool

	cmd := &cobra.Command{
		Use:       "export <shell>",
		Short:     "Export environment variables for the current directory",
		Long:      `Evaluate .envrc files and output shell commands to set environment variables.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			shellName := args[0]

			sh := shell.Get(shellName)
			if sh == nil {
				return fmt.Errorf("unsupported shell: %s (supported: %v)", shellName, shell.Supported())
			}

			return runExport(cmd, sh, stdlib, noCache)
		},
	}

	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Disable evaluation caching")

	return cmd
}

func runExport(cmd *cobra.Command, sh shell.Shell, stdlib string, noCache bool) error {
	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()

	// Get current environment
	currentEnv := env.FromGoEnv(os.Environ())

	// Check for previous state in CASCADE_DIFF
	prevDiffStr := os.Getenv("CASCADE_DIFF")
	var prevDiff *env.EnvDiff
	if prevDiffStr != "" {
		var err error
		prevDiff, err = env.Unmarshal(prevDiffStr)
		if err != nil {
			fmt.Fprintf(stderr, "cascade: warning: invalid CASCADE_DIFF, ignoring: %v\n", err)
			prevDiff = nil
		}
	}

	// Get cascade root for chain traversal (from config or default to home)
	home, err := cfg.GetCascadeRoot()
	if err != nil {
		return fmt.Errorf("get cascade root: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Find .envrc chain from home to cwd
	chain, err := envrc.FindChain(home, cwd)
	if err != nil {
		// If cwd is not under home, just use cwd itself
		chain, err = envrc.FindChain(cwd, cwd)
		if err != nil {
			return fmt.Errorf("find envrc chain: %w", err)
		}
	}

	// Filter to existing files only
	existing := envrc.ExistingOnly(chain)

	// If no .envrc files and we have previous state, revert
	if len(existing) == 0 {
		return handleNoEnvrc(stdout, stderr, sh, prevDiff, nil, nil)
	}

	// Create allow store
	store, err := allow.NewStore()
	if err != nil {
		return fmt.Errorf("create allow store: %w", err)
	}

	// Check allow status for each file (considering whitelist from config)
	var notAllowed []*envrc.RC
	var denied []*envrc.RC
	var allowed []*envrc.RC

	for _, rc := range existing {
		switch store.CheckWithWhitelist(rc, cfg) {
		case allow.Allowed:
			allowed = append(allowed, rc)
		case allow.NotAllowed:
			notAllowed = append(notAllowed, rc)
		case allow.Denied:
			denied = append(denied, rc)
		}
	}

	// If any denied, print error and revert
	if len(denied) > 0 {
		// Create state store for potential recovery
		stateStore, _ := state.NewStore() // Ignore error - best effort

		deniedPaths := make([]string, len(denied))
		for i, rc := range denied {
			fmt.Fprintf(stderr, "cascade: error: %s is blocked. Run `cascade allow %s` to unblock.\n", rc.Path, rc.Path)
			deniedPaths[i] = rc.Path
		}
		return handleNoEnvrc(stdout, stderr, sh, prevDiff, stateStore, deniedPaths)
	}

	// If any not allowed, print warning and skip those
	if len(notAllowed) > 0 {
		for _, rc := range notAllowed {
			fmt.Fprintf(stderr, "cascade: %s is not allowed. Run `cascade allow %s` to allow.\n", rc.Path, rc.Path)
		}
	}

	// If no allowed files, revert
	if len(allowed) == 0 {
		return handleNoEnvrc(stdout, stderr, sh, prevDiff, nil, nil)
	}

	// Get self path for evaluator
	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Create evaluator
	evaluator, err := eval.New("", stdlib, selfPath)
	if err != nil {
		return fmt.Errorf("create evaluator: %w", err)
	}

	// Enable caching unless disabled by flag or config
	cacheEnabled := cfg.CacheEnabled && !noCache
	if cacheEnabled {
		cache, err := eval.NewCache()
		if err != nil {
			// Cache creation failure is not fatal - just log and continue
			fmt.Fprintf(stderr, "cascade: warning: cache unavailable: %v\n", err)
		} else {
			evaluator = evaluator.WithCache(cache)
		}
	}

	// Start with current environment (filtered)
	workingEnv := currentEnv.Filtered()

	// If we have previous state, revert it first to get clean base
	if prevDiff != nil {
		reversed := prevDiff.Reverse()
		workingEnv = reversed.Patch(workingEnv)
	}

	// Evaluate each allowed .envrc in order, accumulating env
	var lastRC *envrc.RC
	var allExtraWatches []string
	for _, rc := range allowed {
		result, err := evaluator.Evaluate(rc, workingEnv)
		if err != nil {
			fmt.Fprintf(stderr, "cascade: error evaluating %s: %v\n", rc.Path, err)
			// Continue with other files? For now, abort and revert
			return handleNoEnvrc(stdout, stderr, sh, prevDiff, nil, nil)
		}
		workingEnv = result.Env
		allExtraWatches = append(allExtraWatches, result.ExtraWatches...)
		lastRC = rc
	}

	// Compute diff from original (reverted) env to final env
	baseEnv := currentEnv.Filtered()
	if prevDiff != nil {
		reversed := prevDiff.Reverse()
		baseEnv = reversed.Patch(baseEnv)
	}
	newDiff := env.BuildEnvDiff(baseEnv, workingEnv)

	// Log environment variable changes if enabled
	// Only log when: directory changed OR diff effect changed (avoids spam on every prompt)
	// Use EqualEffect to compare only Next values - Prev values can differ between runs
	// even when the actual effect (what variables are being set) is identical.
	prevDir := os.Getenv("CASCADE_DIR")
	dirChanged := prevDir != lastRC.Dir
	diffChanged := !newDiff.EqualEffect(prevDiff)
	if cfg.LogEnvDiff && (dirChanged || diffChanged) {
		logEnvDiff(stderr, newDiff, false)
	}

	// Marshal the new diff for CASCADE_DIFF
	diffStr, err := env.Marshal(newDiff)
	if err != nil {
		return fmt.Errorf("marshal diff: %w", err)
	}

	// Build shell export
	export := make(shell.ShellExport)

	// Apply the diff changes
	for key, value := range newDiff.Next {
		if value == "" {
			export.Unset(key)
		} else {
			export.Set(key, value)
		}
	}

	// Set CASCADE_* variables
	if diffStr != "" {
		export.Set("CASCADE_DIFF", diffStr)
	} else {
		// No changes, but we still want to track that we're active
		export.Set("CASCADE_DIFF", "")
	}

	export.Set("CASCADE_DIR", lastRC.Dir)
	export.Set("CASCADE_FILE", lastRC.Path)

	// Build watch list: all .envrc files plus extra watches
	watchPaths := make([]string, 0, len(allowed)+len(allExtraWatches))
	for _, rc := range allowed {
		watchPaths = append(watchPaths, rc.Path)
	}
	watchPaths = append(watchPaths, allExtraWatches...)

	// Serialize and set CASCADE_WATCHES
	watchList := env.NewWatchList(watchPaths)
	if watchStr, err := watchList.Serialize(); err == nil && watchStr != "" {
		export.Set("CASCADE_WATCHES", watchStr)
	}

	// Output shell commands
	fmt.Fprint(stdout, sh.Export(export))

	// Save state for future revert capability
	stateStore, stateErr := state.NewStore()
	if stateErr != nil {
		fmt.Fprintf(stderr, "cascade: warning: state storage unavailable: %v\n", stateErr)
	} else {
		// Save state for the last evaluated .envrc (the leaf of the chain)
		if saveErr := stateStore.Save(lastRC.Path, lastRC.ContentHash, newDiff); saveErr != nil {
			fmt.Fprintf(stderr, "cascade: warning: failed to save state: %v\n", saveErr)
		}
	}

	return nil
}

// handleNoEnvrc handles the case when no .envrc files apply.
// If we have previous state, revert it. Otherwise, do nothing.
func handleNoEnvrc(stdout io.Writer, stderr io.Writer, sh shell.Shell, prevDiff *env.EnvDiff, stateStore *state.Store, deniedPaths []string) error {
	// Try CASCADE_DIFF first
	if prevDiff != nil && !prevDiff.IsEmpty() {
		return revertAndCleanup(stdout, stderr, sh, prevDiff, stateStore, deniedPaths)
	}

	// Fall back to persistent state for denied files
	if stateStore != nil && len(deniedPaths) > 0 {
		for _, path := range deniedPaths {
			if savedState, err := stateStore.Load(path); err == nil && savedState != nil && savedState.Diff != nil {
				return revertAndCleanup(stdout, stderr, sh, savedState.Diff, stateStore, deniedPaths)
			}
		}
	}

	// No state available - warn user if there are denied files
	if len(deniedPaths) > 0 {
		fmt.Fprintf(stderr, "cascade: warning: cannot determine variables set by denied files\n")
		fmt.Fprintf(stderr, "cascade: warning: environment may contain stale variables. Consider restarting your shell.\n")
	}

	return nil
}

// revertAndCleanup reverts the diff and cleans up state files
func revertAndCleanup(stdout, stderr io.Writer, sh shell.Shell, diff *env.EnvDiff, stateStore *state.Store, deniedPaths []string) error {
	// Log environment variable changes if enabled
	if cfg.LogEnvDiff {
		logEnvDiff(stderr, diff, true)
	}

	export := make(shell.ShellExport)

	reversed := diff.Reverse()
	for key, value := range reversed.Next {
		if value == "" {
			export.Unset(key)
		} else {
			export.Set(key, value)
		}
	}

	// Clear CASCADE_* variables
	export.Unset("CASCADE_DIFF")
	export.Unset("CASCADE_DIR")
	export.Unset("CASCADE_FILE")
	export.Unset("CASCADE_WATCHES")

	fmt.Fprint(stdout, sh.Export(export))

	// Clean up state files after successful revert
	if stateStore != nil {
		for _, path := range deniedPaths {
			_ = stateStore.Delete(path)
		}
	}

	return nil
}

// logEnvDiff logs environment variable changes to stderr.
// Format: "cascade export: +VAR -VAR ~VAR" or "cascade unloading: ..."
func logEnvDiff(w io.Writer, diff *env.EnvDiff, unloading bool) {
	if diff == nil || diff.IsEmpty() {
		return
	}

	c := newColorizer(w)
	var parts []string

	// Collect and sort keys for deterministic output
	keys := make([]string, 0, len(diff.Next))
	for k := range diff.Next {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		newVal := diff.Next[key]
		oldVal := diff.Prev[key]

		switch {
		case oldVal == "" && newVal != "": // Added
			parts = append(parts, c.green("+"+key))
		case oldVal != "" && newVal == "": // Removed
			parts = append(parts, c.red("-"+key))
		default: // Changed
			parts = append(parts, c.yellow("~"+key))
		}
	}

	if len(parts) > 0 {
		prefix := "cascade export:"
		if unloading {
			prefix = "cascade unloading:"
		}
		fmt.Fprintf(w, "%s %s\n", prefix, strings.Join(parts, " "))
	}
}

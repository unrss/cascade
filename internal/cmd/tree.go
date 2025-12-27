package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
	"github.com/unrss/cascade/internal/eval"
)

// TreeOutput is the JSON representation of cascade tree.
type TreeOutput struct {
	Root    string      `json:"root"`
	Current string      `json:"current"`
	Levels  []TreeLevel `json:"levels"`
}

// TreeLevel represents a single directory level in the cascade chain.
type TreeLevel struct {
	Path      string     `json:"path"`
	Dir       string     `json:"dir"`
	Exists    bool       `json:"exists"`
	Status    string     `json:"status"` // "allowed", "denied", "not_allowed", "" (if !Exists)
	IsCurrent bool       `json:"is_current"`
	Variables []VarEntry `json:"variables,omitempty"`
}

// VarEntry represents a variable change at a tree level.
type VarEntry struct {
	Name   string `json:"name"`
	Action string `json:"action"` // set, prepend, append, override, modify, unset
	Value  string `json:"value,omitempty"`
}

func newTreeCmd(stdlib string) *cobra.Command {
	var jsonOutput bool
	var showValues bool

	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Show the cascade of .envrc files",
		Long: `Display a tree view of .envrc files in the cascade chain from the
cascade root (typically home directory) to the current directory.

Shows the trust status of each .envrc file:
  - allowed: file is trusted and will be evaluated
  - denied: file is explicitly blocked
  - not allowed: file exists but needs approval

For allowed files, shows which environment variables are set or modified.`,
		Example: `  cascade tree
  cascade tree --json
  cascade tree --values`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTree(cmd.OutOrStdout(), cmd.ErrOrStderr(), stdlib, jsonOutput, showValues)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	cmd.Flags().BoolVarP(&showValues, "values", "v", false, "Show variable values")

	return cmd
}

func runTree(stdout, stderr io.Writer, stdlib string, jsonOutput, showValues bool) error {
	output, err := gatherTree(stderr, stdlib, showValues)
	if err != nil {
		return err
	}

	if jsonOutput {
		return outputTreeJSON(stdout, output)
	}

	return outputTreeHuman(stdout, output, showValues)
}

func gatherTree(stderr io.Writer, stdlib string, showValues bool) (*TreeOutput, error) {
	// Get cascade root for chain traversal (from config or default to home)
	root, err := cfg.GetCascadeRoot()
	if err != nil {
		return nil, fmt.Errorf("get cascade root: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	output := &TreeOutput{
		Root:    root,
		Current: cwd,
		Levels:  []TreeLevel{},
	}

	// Find .envrc chain from root to cwd
	chain, err := envrc.FindChain(root, cwd)
	if err != nil {
		// If cwd is not under root, just use cwd itself
		chain, err = envrc.FindChain(cwd, cwd)
		if err != nil {
			return nil, fmt.Errorf("find envrc chain: %w", err)
		}
		output.Root = cwd
	}

	// Create allow store
	store, err := allow.NewStore()
	if err != nil {
		return nil, fmt.Errorf("create allow store: %w", err)
	}

	// Build levels from chain and collect allowed RCs for evaluation
	var allowedRCs []*envrc.RC
	levelIndices := make(map[string]int) // Map RC path to level index

	for _, rc := range chain {
		level := TreeLevel{
			Path:      rc.Path,
			Dir:       rc.Dir,
			Exists:    rc.Exists,
			IsCurrent: rc.Dir == cwd,
		}

		// Determine status for existing files
		if rc.Exists {
			status := store.CheckWithWhitelist(rc, cfg)
			level.Status = status.String()

			// Track allowed RCs for variable evaluation
			if status == allow.Allowed {
				levelIndices[rc.Path] = len(output.Levels)
				allowedRCs = append(allowedRCs, rc)
			}
		}

		output.Levels = append(output.Levels, level)
	}

	// Evaluate allowed RCs to track variable changes
	if len(allowedRCs) > 0 {
		if err := evaluateVariables(stderr, stdlib, allowedRCs, output, levelIndices, showValues); err != nil {
			// Log warning but don't fail the command
			fmt.Fprintf(stderr, "cascade: warning: error evaluating variables: %v\n", err)
		}
	}

	return output, nil
}

// evaluateVariables evaluates each allowed RC and tracks variable changes.
func evaluateVariables(stderr io.Writer, stdlib string, allowedRCs []*envrc.RC, output *TreeOutput, levelIndices map[string]int, showValues bool) error {
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

	// Start with current environment (filtered)
	currentEnv := env.FromGoEnv(os.Environ())
	workingEnv := currentEnv.Filtered()

	// Evaluate each allowed RC in order, tracking variable changes
	for _, rc := range allowedRCs {
		prevEnv := workingEnv.Copy()

		result, err := evaluator.Evaluate(rc, workingEnv)
		if err != nil {
			fmt.Fprintf(stderr, "cascade: warning: error evaluating %s: %v\n", rc.Path, err)
			continue
		}

		// Find variable changes
		vars := detectVariableChanges(prevEnv, result.Env, showValues)

		// Update the corresponding level
		if idx, ok := levelIndices[rc.Path]; ok {
			output.Levels[idx].Variables = vars
		}

		workingEnv = result.Env
	}

	return nil
}

// detectVariableChanges compares before/after environments and returns variable entries.
func detectVariableChanges(before, after env.Env, showValues bool) []VarEntry {
	// Pre-allocate with reasonable capacity
	entries := make([]VarEntry, 0, len(after))

	// Check for new or modified variables
	for key, newVal := range after {
		oldVal, existed := before[key]

		var entry VarEntry
		entry.Name = key

		if !existed {
			entry.Action = "set"
		} else if newVal != oldVal {
			if treeIsPathLikeVar(key) {
				entry.Action = treeDetectPathAction(oldVal, newVal)
			} else {
				entry.Action = "override"
			}
		} else {
			// No change
			continue
		}

		if showValues {
			entry.Value = newVal
		}

		entries = append(entries, entry)
	}

	// Check for unset variables
	for key := range before {
		if _, exists := after[key]; !exists {
			entry := VarEntry{
				Name:   key,
				Action: "unset",
			}
			entries = append(entries, entry)
		}
	}

	// Sort entries by name for consistent output
	slices.SortFunc(entries, func(a, b VarEntry) int {
		return strings.Compare(a.Name, b.Name)
	})

	return entries
}

// treeIsPathLikeVar returns true if the variable is typically a colon-separated path.
// Duplicated from which.go to avoid exporting internal helpers.
func treeIsPathLikeVar(name string) bool {
	pathVars := map[string]bool{
		"PATH":            true,
		"MANPATH":         true,
		"INFOPATH":        true,
		"LD_LIBRARY_PATH": true,
		"LIBRARY_PATH":    true,
		"CPATH":           true,
		"PKG_CONFIG_PATH": true,
		"PYTHONPATH":      true,
		"GOPATH":          true,
		"NODE_PATH":       true,
		"CLASSPATH":       true,
		"CDPATH":          true,
	}
	return pathVars[name]
}

// treeDetectPathAction determines if a path was prepended, appended, or replaced.
// Duplicated from which.go to avoid exporting internal helpers.
func treeDetectPathAction(oldValue, newValue string) string {
	if oldValue == "" {
		return "set"
	}

	// Check if old value is a suffix (new value was prepended)
	if strings.HasSuffix(newValue, ":"+oldValue) {
		return "prepend"
	}

	// Check if old value is a prefix (new value was appended)
	if strings.HasPrefix(newValue, oldValue+":") {
		return "append"
	}

	// Check if old value is contained (both prepend and append happened)
	if strings.Contains(newValue, ":"+oldValue+":") {
		return "modify"
	}

	// Value was completely replaced
	return "override"
}

func outputTreeJSON(w io.Writer, output *TreeOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputTreeHuman(w io.Writer, output *TreeOutput, showValues bool) error {
	c := newColorizer(w)

	// Get home directory for path shortening
	home, _ := os.UserHomeDir()

	// Filter to only existing .envrc files for display
	var existingLevels []TreeLevel
	for _, level := range output.Levels {
		if level.Exists {
			existingLevels = append(existingLevels, level)
		}
	}

	if len(existingLevels) == 0 {
		fmt.Fprintf(w, "%s\n", c.dim("No .envrc files found in cascade chain"))
		return nil
	}

	// Render each level
	for _, level := range existingLevels {
		displayDir := shortenPath(level.Dir, home)

		// Add current marker
		if level.IsCurrent {
			displayDir += " " + c.dim("<- current")
		}

		// Print directory path
		fmt.Fprintln(w, displayDir)

		// Print .envrc line with status
		var icon, statusText string
		switch level.Status {
		case "allowed":
			icon = c.green("\u2713")
			statusText = c.green("allowed")
		case "denied":
			icon = c.red("\u2717")
			statusText = c.red("denied")
		case "not allowed":
			icon = c.yellow("\u26a0")
			statusText = c.yellow("not allowed")
		default:
			icon = "?"
			statusText = level.Status
		}

		// Determine if we have variables to show
		hasVars := len(level.Variables) > 0

		// Use different tree characters based on whether we have variables
		if hasVars {
			fmt.Fprintf(w, "\u251c\u2500\u2500 %s %s %s\n", filepath.Base(level.Path), icon, statusText)
			renderVariables(w, c, level.Variables, showValues, home)
		} else {
			fmt.Fprintf(w, "\u2514\u2500\u2500 %s %s %s\n", filepath.Base(level.Path), icon, statusText)
		}
		fmt.Fprintln(w)
	}

	return nil
}

// renderVariables renders the variable entries under a tree level.
func renderVariables(w io.Writer, c *colorizer, vars []VarEntry, showValues bool, home string) {
	for i, v := range vars {
		isLast := i == len(vars)-1

		// Tree connector
		var connector string
		if isLast {
			connector = "\u2514\u2500\u2500"
		} else {
			connector = "\u251c\u2500\u2500"
		}

		// Format action symbol
		actionSymbol := formatActionSymbol(v.Action)

		// Build the line
		if showValues && v.Value != "" {
			displayValue := v.Value
			// Shorten paths in values
			if treeIsPathLikeVar(v.Name) {
				displayValue = shortenPathList(displayValue, home)
			} else {
				displayValue = shortenPath(displayValue, home)
			}
			// Truncate long values
			if len(displayValue) > 60 {
				displayValue = displayValue[:57] + "..."
			}
			fmt.Fprintf(w, "\u2502   %s %s %s %s\n", connector, c.cyan(v.Name), c.dim(actionSymbol), c.dim(displayValue))
		} else {
			fmt.Fprintf(w, "\u2502   %s %s %s\n", connector, c.cyan(v.Name), c.dim(actionSymbol))
		}
	}
}

// formatActionSymbol returns a symbol representing the action.
func formatActionSymbol(action string) string {
	switch action {
	case "set":
		return "="
	case "prepend":
		return "+="
	case "append":
		return "=+"
	case "override":
		return ":="
	case "modify":
		return "~="
	case "unset":
		return "x"
	default:
		return "?"
	}
}

// shortenPathList shortens each path in a colon-separated list.
func shortenPathList(pathList, home string) string {
	parts := filepath.SplitList(pathList)
	for i, part := range parts {
		parts[i] = shortenPath(part, home)
	}
	return strings.Join(parts, ":")
}

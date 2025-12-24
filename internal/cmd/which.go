package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
	"github.com/unrss/cascade/internal/eval"
)

// WhichOutput is the JSON representation of cascade which.
type WhichOutput struct {
	Variable string       `json:"variable"`
	Value    string       `json:"value,omitempty"`
	SetBy    []SetByEntry `json:"set_by,omitempty"`
	NotFound bool         `json:"not_found,omitempty"`
}

// SetByEntry represents a single .envrc file that set or modified a variable.
type SetByEntry struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "set", "append", "prepend", "override"
}

func newWhichCmd(stdlib string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "which VAR",
		Short: "Show which .envrc file set a variable",
		Long: `Show which .envrc file(s) set or modified the specified environment variable.

For path-like variables (PATH, MANPATH, etc.), shows which files added entries.
For regular variables, shows which file set the value and any overrides.`,
		Example: `  cascade which PATH
  cascade which MY_VAR
  cascade which --json PATH`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWhich(cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], stdlib, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func runWhich(stdout, stderr io.Writer, varName, stdlib string, jsonOutput bool) error {
	output, err := gatherWhich(stderr, varName, stdlib)
	if err != nil {
		return err
	}

	if jsonOutput {
		return outputWhichJSON(stdout, output)
	}

	return outputWhichHuman(stdout, output)
}

func gatherWhich(stderr io.Writer, varName, stdlib string) (*WhichOutput, error) {
	output := &WhichOutput{
		Variable: varName,
		SetBy:    []SetByEntry{},
	}

	// Get home directory for chain root
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	// Find .envrc chain from home to cwd
	chain, err := envrc.FindChain(home, cwd)
	if err != nil {
		// If cwd is not under home, just use cwd itself
		chain, err = envrc.FindChain(cwd, cwd)
		if err != nil {
			return nil, fmt.Errorf("find envrc chain: %w", err)
		}
	}

	// Filter to existing files only
	existing := envrc.ExistingOnly(chain)

	if len(existing) == 0 {
		output.NotFound = true
		return output, nil
	}

	// Create allow store
	store, err := allow.NewStore()
	if err != nil {
		return nil, fmt.Errorf("create allow store: %w", err)
	}

	// Filter to allowed files only
	var allowed []*envrc.RC
	for _, rc := range existing {
		if store.Check(rc) == allow.Allowed {
			allowed = append(allowed, rc)
		}
	}

	if len(allowed) == 0 {
		output.NotFound = true
		return output, nil
	}

	// Get self path for evaluator
	selfPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("get executable path: %w", err)
	}

	// Create evaluator
	evaluator, err := eval.New("", stdlib, selfPath)
	if err != nil {
		return nil, fmt.Errorf("create evaluator: %w", err)
	}

	// Start with current environment (filtered)
	currentEnv := env.FromGoEnv(os.Environ())
	workingEnv := currentEnv.Filtered()

	// Track the variable value before and after each .envrc
	isPathLike := isPathLikeVar(varName)
	var prevValue string

	// Evaluate each allowed .envrc in order, tracking changes to the variable
	for _, rc := range allowed {
		prevValue = workingEnv[varName]

		result, err := evaluator.Evaluate(rc, workingEnv)
		if err != nil {
			fmt.Fprintf(stderr, "cascade: warning: error evaluating %s: %v\n", rc.Path, err)
			continue
		}

		newValue := result.Env[varName]
		workingEnv = result.Env

		// Check if this file changed the variable
		if newValue != prevValue {
			entry := SetByEntry{Path: rc.Path}

			if isPathLike {
				entry.Action = detectPathAction(prevValue, newValue)
			} else {
				if prevValue == "" {
					entry.Action = "set"
				} else {
					entry.Action = "override"
				}
			}

			output.SetBy = append(output.SetBy, entry)
		}
	}

	// Set the final value
	output.Value = workingEnv[varName]

	// If no .envrc set this variable, mark as not found
	if len(output.SetBy) == 0 {
		output.NotFound = true
	}

	return output, nil
}

// isPathLikeVar returns true if the variable is typically a colon-separated path.
func isPathLikeVar(name string) bool {
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

// detectPathAction determines if a path was prepended, appended, or replaced.
func detectPathAction(oldValue, newValue string) string {
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

func outputWhichJSON(w io.Writer, output *WhichOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputWhichHuman(w io.Writer, output *WhichOutput) error {
	c := newColorizer(w)

	// Get home directory for path shortening
	home, _ := os.UserHomeDir()

	if output.NotFound {
		fmt.Fprintf(w, "%s is not set by any .envrc file\n", c.bold(output.Variable))
		if output.Value != "" {
			fmt.Fprintf(w, "%s\n", c.dim("(set by shell or system)"))
		}
		return nil
	}

	// Show which files set the variable
	if len(output.SetBy) == 1 {
		fmt.Fprintf(w, "%s is set by:\n", c.bold(output.Variable))
	} else {
		fmt.Fprintf(w, "%s is set by multiple files:\n", c.bold(output.Variable))
	}

	for i, entry := range output.SetBy {
		displayPath := shortenPath(entry.Path, home)
		actionDesc := formatAction(entry.Action, i == 0)
		fmt.Fprintf(w, "  %s  %s\n", displayPath, c.dim("("+actionDesc+")"))
	}

	fmt.Fprintln(w)

	// Show the current value
	if isPathLikeVar(output.Variable) {
		fmt.Fprintf(w, "%s\n", c.bold("Current value:"))
		// Split path and show each entry on its own line
		parts := filepath.SplitList(output.Value)
		for _, part := range parts {
			displayPart := shortenPath(part, home)
			fmt.Fprintf(w, "  %s\n", displayPart)
		}
	} else {
		fmt.Fprintf(w, "%s %s\n", c.bold("Value:"), formatValue(output.Value))
	}

	return nil
}

// formatAction returns a human-readable description of the action.
func formatAction(action string, isFirst bool) string {
	switch action {
	case "set":
		if isFirst {
			return "base value"
		}
		return "set"
	case "prepend":
		return "prepended"
	case "append":
		return "appended"
	case "override":
		return "overrides"
	case "modify":
		return "modified"
	default:
		return action
	}
}

// formatValue formats a value for display, quoting if it contains spaces.
func formatValue(value string) string {
	if strings.ContainsAny(value, " \t\n") {
		return fmt.Sprintf("%q", value)
	}
	return value
}

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/envrc"
)

// TreeOutput is the JSON representation of cascade tree.
type TreeOutput struct {
	Root    string      `json:"root"`
	Current string      `json:"current"`
	Levels  []TreeLevel `json:"levels"`
}

// TreeLevel represents a single directory level in the cascade chain.
type TreeLevel struct {
	Path      string `json:"path"`
	Dir       string `json:"dir"`
	Exists    bool   `json:"exists"`
	Status    string `json:"status"` // "allowed", "denied", "not_allowed", "" (if !Exists)
	IsCurrent bool   `json:"is_current"`
}

func newTreeCmd(_ string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Show the cascade of .envrc files",
		Long: `Display a tree view of .envrc files in the cascade chain from the
cascade root (typically home directory) to the current directory.

Shows the trust status of each .envrc file:
  - allowed: file is trusted and will be evaluated
  - denied: file is explicitly blocked
  - not allowed: file exists but needs approval`,
		Example: `  cascade tree
  cascade tree --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTree(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func runTree(stdout, _ io.Writer, jsonOutput bool) error {
	output, err := gatherTree()
	if err != nil {
		return err
	}

	if jsonOutput {
		return outputTreeJSON(stdout, output)
	}

	return outputTreeHuman(stdout, output)
}

func gatherTree() (*TreeOutput, error) {
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

	// Build levels from chain
	for _, rc := range chain {
		level := TreeLevel{
			Path:      rc.Path,
			Dir:       rc.Dir,
			Exists:    rc.Exists,
			IsCurrent: rc.Dir == cwd,
		}

		// Determine status for existing files
		if rc.Exists {
			level.Status = store.CheckWithWhitelist(rc, cfg).String()
		}

		output.Levels = append(output.Levels, level)
	}

	return output, nil
}

func outputTreeJSON(w io.Writer, output *TreeOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputTreeHuman(w io.Writer, output *TreeOutput) error {
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

		fmt.Fprintf(w, "\u2514\u2500\u2500 %s %s %s\n", filepath.Base(level.Path), icon, statusText)
		fmt.Fprintln(w)
	}

	return nil
}

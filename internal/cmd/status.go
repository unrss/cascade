package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/envrc"
)

// StatusOutput is the JSON representation of cascade status.
type StatusOutput struct {
	Active          bool              `json:"active"`
	Directory       string            `json:"directory,omitempty"`
	Chain           []ChainEntry      `json:"chain"`
	Variables       map[string]string `json:"variables,omitempty"`
	Watches         []WatchEntry      `json:"watches,omitempty"`
	TrustedSubtrees []string          `json:"trusted_subtrees,omitempty"`
}

// ChainEntry represents a single .envrc file in the chain.
type ChainEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Status string `json:"status"` // "allowed", "denied", "not_allowed"
}

// WatchEntry represents a watched file.
type WatchEntry struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Changed bool   `json:"changed"`
	Extra   bool   `json:"extra,omitempty"` // True if added via watch_file (not an .envrc)
}

func newStatusCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show cascade status for the current directory",
		Long:  `Display the current cascade state including loaded .envrc files and environment changes.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.OutOrStdout(), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func runStatus(w io.Writer, jsonOutput bool) error {
	status, err := gatherStatus()
	if err != nil {
		return err
	}

	if jsonOutput {
		return outputJSON(w, status)
	}

	return outputHuman(w, status)
}

func gatherStatus() (*StatusOutput, error) {
	status := &StatusOutput{
		Chain:     []ChainEntry{},
		Variables: make(map[string]string),
		Watches:   []WatchEntry{},
	}

	// Check if cascade is active
	cascadeDir := os.Getenv("CASCADE_DIR")
	status.Active = cascadeDir != ""
	status.Directory = cascadeDir

	// Get cascade root for chain traversal (from config or default to home)
	home, err := cfg.GetCascadeRoot()
	if err != nil {
		return nil, fmt.Errorf("get cascade root: %w", err)
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

	// Create allow store
	store, err := allow.NewStore()
	if err != nil {
		return nil, fmt.Errorf("create allow store: %w", err)
	}

	// Build chain entries (existing files only for display)
	for _, rc := range chain {
		if !rc.Exists {
			continue
		}

		entry := ChainEntry{
			Path:   rc.Path,
			Exists: rc.Exists,
			Status: store.CheckWithWhitelist(rc, cfg).String(),
		}
		status.Chain = append(status.Chain, entry)
	}

	// Parse CASCADE_DIFF to get variables
	cascadeDiff := os.Getenv("CASCADE_DIFF")
	if cascadeDiff != "" {
		diff, err := env.Unmarshal(cascadeDiff)
		if err == nil && diff != nil {
			for k, v := range diff.Next {
				if v != "" { // Only include set variables, not deletions
					status.Variables[k] = v
				}
			}
		}
	}

	// Build set of .envrc paths for identifying extra watches
	envrcPaths := make(map[string]bool)
	for _, entry := range status.Chain {
		envrcPaths[entry.Path] = true
	}

	// Parse CASCADE_WATCHES to get watched files
	cascadeWatches := os.Getenv("CASCADE_WATCHES")
	if cascadeWatches != "" {
		watchList, err := env.ParseWatchList(cascadeWatches)
		if err == nil {
			for _, ft := range watchList {
				entry := WatchEntry{
					Path:    ft.Path,
					Exists:  ft.Exists,
					Changed: ft.Check(),
					Extra:   !envrcPaths[ft.Path], // Extra if not an .envrc file
				}
				status.Watches = append(status.Watches, entry)
			}
		}
	}

	// Get trusted subtrees
	trustedPaths, err := store.ListTrustedSubtrees()
	if err == nil && len(trustedPaths) > 0 {
		sort.Strings(trustedPaths)
		status.TrustedSubtrees = trustedPaths
	}

	return status, nil
}

func outputJSON(w io.Writer, status *StatusOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func outputHuman(w io.Writer, status *StatusOutput) error {
	c := newColorizer(w)

	// Get home directory for path shortening
	home, _ := os.UserHomeDir()

	// Active state
	if status.Active {
		fmt.Fprintf(w, "%s\n", c.bold("Cascade is active"))
		fmt.Fprintf(w, "  Directory: %s\n", status.Directory)
	} else {
		fmt.Fprintf(w, "%s\n", c.dim("Cascade is not active"))
	}
	fmt.Fprintln(w)

	// .envrc chain
	if len(status.Chain) > 0 {
		fmt.Fprintf(w, "%s\n", c.bold(".envrc chain:"))
		for _, entry := range status.Chain {
			displayPath := shortenPath(entry.Path, home)

			var icon, statusText string
			switch entry.Status {
			case "allowed":
				icon = c.green("✓")
				statusText = c.green("allowed")
			case "denied":
				icon = c.red("✗")
				statusText = c.red("denied")
			case "not allowed":
				icon = c.yellow("⚠")
				statusText = c.yellow("not allowed")
			default:
				icon = "?"
				statusText = entry.Status
			}

			fmt.Fprintf(w, "  %s %s (%s)\n", icon, displayPath, statusText)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "%s\n\n", c.dim("No .envrc files found"))
	}

	// Variables set (only if cascade is active and has variables)
	if status.Active && len(status.Variables) > 0 {
		fmt.Fprintf(w, "%s\n", c.bold("Variables set:"))

		// Sort variable names for consistent output
		varNames := make([]string, 0, len(status.Variables))
		for name := range status.Variables {
			varNames = append(varNames, name)
		}
		sort.Strings(varNames)

		// Find max variable name length for alignment
		maxLen := 0
		for _, name := range varNames {
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}

		for _, name := range varNames {
			// Truncate long values for display
			value := status.Variables[name]
			displayValue := truncateValue(value, 50)
			fmt.Fprintf(w, "  %-*s = %s\n", maxLen, name, displayValue)
		}
		fmt.Fprintln(w)
	}

	// Watched files (only if cascade is active and has watches)
	if status.Active && len(status.Watches) > 0 {
		fmt.Fprintf(w, "%s\n", c.bold("Watched files:"))
		for _, watch := range status.Watches {
			displayPath := shortenPath(watch.Path, home)

			var changeStatus string
			if watch.Changed {
				changeStatus = c.yellow("changed")
			} else {
				changeStatus = c.dim("unchanged")
			}

			if watch.Extra {
				fmt.Fprintf(w, "  %s (%s - %s)\n", displayPath, c.dim("extra"), changeStatus)
			} else {
				fmt.Fprintf(w, "  %s (%s)\n", displayPath, changeStatus)
			}
		}
		fmt.Fprintln(w)
	}

	// Trusted subtrees
	if len(status.TrustedSubtrees) > 0 {
		fmt.Fprintf(w, "%s\n", c.bold("Trusted subtrees:"))
		for _, p := range status.TrustedSubtrees {
			displayPath := shortenPath(p, home)
			fmt.Fprintf(w, "  %s\n", displayPath)
		}
	}

	return nil
}

// shortenPath replaces home directory prefix with ~
func shortenPath(path, home string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && !filepath.IsAbs(rel) {
			return "~/" + rel
		}
	}
	return path
}

// truncateValue shortens long values for display
func truncateValue(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen-3] + "..."
}

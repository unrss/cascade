package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
)

func newTrustCmd() *cobra.Command {
	var (
		list   bool
		remove bool
	)

	cmd := &cobra.Command{
		Use:   "trust [path]",
		Short: "Trust all .envrc files under a directory",
		Long: `Mark a directory subtree as trusted, allowing all .envrc files
under it to be evaluated without individual approval.

Examples:
  cascade trust ~/work          # Trust all .envrc files under ~/work
  cascade trust --list          # List all trusted subtrees
  cascade trust --remove ~/work # Remove trust for ~/work`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := allow.NewStore()
			if err != nil {
				return fmt.Errorf("create allow store: %w", err)
			}

			if list {
				return runTrustList(cmd, store)
			}

			if remove {
				return runTrustRemove(cmd, args, store)
			}

			return runTrustAdd(cmd, args, store)
		},
	}

	cmd.Flags().BoolVarP(&list, "list", "l", false, "List all trusted subtrees")
	cmd.Flags().BoolVarP(&remove, "remove", "d", false, "Remove trust for a subtree")

	return cmd
}

func runTrustAdd(cmd *cobra.Command, args []string, store *allow.Store) error {
	if len(args) == 0 {
		return errors.New("path required")
	}

	path := args[0]
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	if err := store.TrustSubtree(absPath); err != nil {
		return fmt.Errorf("trust subtree: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "cascade: trusted subtree %s\n", absPath)
	return nil
}

func runTrustRemove(cmd *cobra.Command, args []string, store *allow.Store) error {
	if len(args) == 0 {
		return errors.New("path required")
	}

	path := args[0]
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	if err := store.UntrustSubtree(absPath); err != nil {
		return fmt.Errorf("untrust subtree: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "cascade: removed trust for %s\n", absPath)
	return nil
}

func runTrustList(cmd *cobra.Command, store *allow.Store) error {
	paths, err := store.ListTrustedSubtrees()
	if err != nil {
		return fmt.Errorf("list trusted subtrees: %w", err)
	}

	if len(paths) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No trusted subtrees")
		return nil
	}

	// Sort for consistent output
	sort.Strings(paths)

	// Get home directory for path shortening
	home, _ := os.UserHomeDir()

	fmt.Fprintln(cmd.OutOrStdout(), "Trusted subtrees:")
	for _, p := range paths {
		displayPath := shortenPathForDisplay(p, home)
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", displayPath)
	}

	return nil
}

// shortenPathForDisplay replaces home directory prefix with ~
func shortenPathForDisplay(path, home string) string {
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && !filepath.IsAbs(rel) {
			return "~/" + rel
		}
	}
	return path
}

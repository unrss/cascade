package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/envrc"
)

func newAllowCmd() *cobra.Command {
	var recursive bool

	cmd := &cobra.Command{
		Use:   "allow [path]",
		Short: "Allow an .envrc file to be loaded",
		Long: `Mark an .envrc file as trusted, allowing it to be evaluated.
If no path is provided, defaults to ./.envrc in the current directory.

Use --recursive to trust all .envrc files under a directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Create allow store
			store, err := allow.NewStore()
			if err != nil {
				return fmt.Errorf("create allow store: %w", err)
			}

			if recursive {
				return runAllowRecursive(cmd, args, store)
			}
			return runAllowSingle(cmd, args, store)
		},
	}

	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false,
		"Trust all .envrc files under this directory")

	return cmd
}

func runAllowSingle(cmd *cobra.Command, args []string, store *allow.Store) error {
	path := ".envrc"
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Create RC to validate file exists and compute hash
	rc, err := envrc.NewRC(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	if !rc.Exists {
		return fmt.Errorf("file does not exist: %s", absPath)
	}

	// Allow the file
	if err := store.Allow(rc); err != nil {
		return fmt.Errorf("allow file: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "cascade: allowed %s\n", rc.Path)
	return nil
}

func runAllowRecursive(cmd *cobra.Command, args []string, store *allow.Store) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Trust the subtree
	if err := store.TrustSubtree(absPath); err != nil {
		return fmt.Errorf("trust subtree: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "cascade: trusted subtree %s\n", absPath)
	return nil
}

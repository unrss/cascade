package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/envrc"
)

func newDenyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deny [path]",
		Short: "Deny an .envrc file from being loaded",
		Long: `Revoke trust for an .envrc file, preventing it from being evaluated.
If no path is provided, defaults to ./.envrc in the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := ".envrc"
			if len(args) > 0 {
				path = args[0]
			}

			// Resolve to absolute path
			absPath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve path: %w", err)
			}

			// Create RC - file doesn't need to exist for deny
			rc, err := envrc.NewRC(absPath)
			if err != nil {
				return fmt.Errorf("read file: %w", err)
			}

			// Create allow store
			store, err := allow.NewStore()
			if err != nil {
				return fmt.Errorf("create allow store: %w", err)
			}

			// Deny the file
			if err := store.Deny(rc); err != nil {
				return fmt.Errorf("deny file: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "cascade: denied %s\n", rc.Path)
			return nil
		},
	}
}

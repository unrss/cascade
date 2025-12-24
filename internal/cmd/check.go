// Package cmd implements the cascade CLI commands.
package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/envrc"
)

func newCheckCmd() *cobra.Command {
	var silent bool

	cmd := &cobra.Command{
		Use:   "check <file>",
		Short: "Check if an envrc file is allowed",
		Long: `Check the allow status of a specific .envrc file.

Returns exit code 0 if allowed, 1 if not allowed or denied.
Use --silent for scripting (no output, exit code only).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd.OutOrStdout(), cmd.ErrOrStderr(), args[0], silent)
		},
	}

	cmd.Flags().BoolVarP(&silent, "silent", "s", false, "suppress output (exit code only)")

	return cmd
}

func runCheck(stdout, stderr io.Writer, path string, silent bool) error {
	rc, err := envrc.NewRC(path)
	if err != nil {
		if !silent {
			fmt.Fprintf(stderr, "error: %v\n", err)
		}
		return err
	}

	store, err := allow.NewStore()
	if err != nil {
		if !silent {
			fmt.Fprintf(stderr, "error: %v\n", err)
		}
		return err
	}

	status := store.CheckWithWhitelist(rc, cfg)

	switch status {
	case allow.Allowed:
		if !silent {
			fmt.Fprintf(stdout, "allowed: %s\n", rc.Path)
		}
		return nil
	case allow.NotAllowed:
		if !silent {
			fmt.Fprintf(stdout, "not allowed: %s\n", rc.Path)
		}
		return errors.New("not allowed")
	case allow.Denied:
		if !silent {
			fmt.Fprintf(stdout, "denied: %s\n", rc.Path)
		}
		return errors.New("denied")
	default:
		return fmt.Errorf("unknown status: %v", status)
	}
}

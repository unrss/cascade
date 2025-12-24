package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/env"
	"github.com/unrss/cascade/internal/eval"
)

func newDumpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "dump",
		Short:  "Dump environment in various formats",
		Long:   `Output the current environment in the specified format. Used internally by stdlib.sh.`,
		Hidden: true, // Internal command
	}

	cmd.AddCommand(newDumpJSONCmd())

	return cmd
}

func newDumpJSONCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "json",
		Short: "Dump environment as JSON",
		Long:  `Output the current environment as JSON. Called by stdlib.sh __dump_at_exit trap.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			currentEnv := env.FromGoEnv(os.Environ())

			if err := eval.DumpJSON(currentEnv, cmd.OutOrStdout()); err != nil {
				return fmt.Errorf("dump json: %w", err)
			}

			return nil
		},
	}
}

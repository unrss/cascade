// Package cmd implements the cascade CLI commands.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/config"
)

// Assets holds embedded files passed from main.
type Assets struct {
	Stdlib  string
	Version string
}

// cfg holds the loaded configuration, available to all commands.
var cfg *config.Config

// Execute runs the root command with the provided assets.
func Execute(assets Assets) error {
	root := newRootCmd(assets)
	return root.Execute()
}

func newRootCmd(assets Assets) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cascade",
		Short: "Hierarchical environment variable management",
		Long: `cascade is a direnv-like tool for managing environment variables
with hierarchical inheritance across directories.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initConfig()
		},
	}

	// Add subcommands
	cmd.AddCommand(
		newHookCmd(),
		newExportCmd(assets.Stdlib),
		newAllowCmd(),
		newDenyCmd(),
		newTrustCmd(),
		newStatusCmd(),
		newCheckCmd(),
		newVersionCmd(assets.Version),
		newDumpCmd(),
		newWhichCmd(assets.Stdlib),
		newConfigCmd(),
		newMigrateCmd(),
	)

	return cmd
}

func initConfig() error {
	var err error
	cfg, err = config.Load()
	return err
}

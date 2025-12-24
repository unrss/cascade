package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/shell"
)

func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "hook <shell>",
		Short:     "Print shell hook for cascade integration",
		Long:      `Print the shell hook that should be evaluated in your shell's rc file.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			shellName := args[0]

			sh := shell.Get(shellName)
			if sh == nil {
				return fmt.Errorf("unsupported shell: %s (supported: %v)", shellName, shell.Supported())
			}

			selfPath, err := os.Executable()
			if err != nil {
				return fmt.Errorf("get executable path: %w", err)
			}

			fmt.Fprint(cmd.OutOrStdout(), sh.Hook(selfPath))
			return nil
		},
	}
}

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/unrss/cascade/internal/config"
)

// ConfigOutput is the JSON representation of cascade configuration.
type ConfigOutput struct {
	ConfigFile      string   `json:"config_file,omitempty"`
	WhitelistPrefix []string `json:"whitelist_prefix,omitempty"`
	BashPath        string   `json:"bash_path,omitempty"`
	DisabledShells  []string `json:"disabled_shells,omitempty"`
	CascadeRoot     string   `json:"cascade_root,omitempty"`
	CacheEnabled    bool     `json:"cache_enabled"`
}

func newConfigCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show current configuration",
		Long: `Display the current cascade configuration including values from
the config file, environment variables, and defaults.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(cmd.OutOrStdout(), jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")

	return cmd
}

func runConfig(w io.Writer, jsonOutput bool) error {
	output := ConfigOutput{
		ConfigFile:      config.ConfigFile(),
		WhitelistPrefix: cfg.WhitelistPrefix,
		BashPath:        cfg.BashPath,
		DisabledShells:  cfg.DisabledShells,
		CascadeRoot:     cfg.CascadeRoot,
		CacheEnabled:    cfg.CacheEnabled,
	}

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	return outputConfigHuman(w, output)
}

func outputConfigHuman(w io.Writer, output ConfigOutput) error {
	c := newConfigColorizer(w)

	fmt.Fprintf(w, "%s\n\n", c.bold("Cascade Configuration"))

	// Config file
	if output.ConfigFile != "" {
		fmt.Fprintf(w, "  %s %s\n", c.label("Config file:"), output.ConfigFile)
	} else {
		fmt.Fprintf(w, "  %s %s\n", c.label("Config file:"), c.dim("(none)"))
	}

	// Whitelist prefixes
	fmt.Fprintf(w, "  %s", c.label("Whitelist prefixes:"))
	if len(output.WhitelistPrefix) == 0 {
		fmt.Fprintf(w, " %s\n", c.dim("(none)"))
	} else {
		fmt.Fprintln(w)
		for _, prefix := range output.WhitelistPrefix {
			fmt.Fprintf(w, "    - %s\n", prefix)
		}
	}

	// Bash path
	fmt.Fprintf(w, "  %s", c.label("Bash path:"))
	if output.BashPath != "" {
		fmt.Fprintf(w, " %s\n", output.BashPath)
	} else {
		fmt.Fprintf(w, " %s\n", c.dim("(auto-detect)"))
	}

	// Disabled shells
	fmt.Fprintf(w, "  %s", c.label("Disabled shells:"))
	if len(output.DisabledShells) == 0 {
		fmt.Fprintf(w, " %s\n", c.dim("(none)"))
	} else {
		fmt.Fprintf(w, " %s\n", strings.Join(output.DisabledShells, ", "))
	}

	// Cascade root
	fmt.Fprintf(w, "  %s", c.label("Cascade root:"))
	if output.CascadeRoot != "" {
		fmt.Fprintf(w, " %s\n", output.CascadeRoot)
	} else {
		fmt.Fprintf(w, " %s\n", c.dim("(default: $HOME)"))
	}

	// Cache enabled
	fmt.Fprintf(w, "  %s", c.label("Cache enabled:"))
	if output.CacheEnabled {
		fmt.Fprintf(w, " %s\n", c.green("true"))
	} else {
		fmt.Fprintf(w, " %s\n", c.yellow("false"))
	}

	return nil
}

// configColorizer handles terminal color output for config command.
type configColorizer struct {
	enabled bool
}

func newConfigColorizer(w io.Writer) *configColorizer {
	enabled := false
	if f, ok := w.(*os.File); ok {
		enabled = term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == ""
	}
	return &configColorizer{enabled: enabled}
}

func (c *configColorizer) bold(s string) string {
	if c.enabled {
		return "\033[1m" + s + "\033[0m"
	}
	return s
}

func (c *configColorizer) dim(s string) string {
	if c.enabled {
		return "\033[2m" + s + "\033[0m"
	}
	return s
}

func (c *configColorizer) label(s string) string {
	if c.enabled {
		return "\033[36m" + s + "\033[0m" // cyan
	}
	return s
}

func (c *configColorizer) green(s string) string {
	if c.enabled {
		return "\033[32m" + s + "\033[0m"
	}
	return s
}

func (c *configColorizer) yellow(s string) string {
	if c.enabled {
		return "\033[33m" + s + "\033[0m"
	}
	return s
}

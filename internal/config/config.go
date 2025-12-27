// Package config manages cascade configuration from files and environment.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds cascade configuration.
type Config struct {
	// WhitelistPrefix contains directory prefixes where .envrc files are auto-allowed.
	WhitelistPrefix []string `mapstructure:"whitelist_prefix"`

	// BashPath is the path to the bash binary. Empty means find via PATH.
	BashPath string `mapstructure:"bash_path"`

	// DisabledShells lists shells that should not be supported.
	DisabledShells []string `mapstructure:"disabled_shells"`

	// CascadeRoot overrides the root directory for .envrc chain traversal.
	// Defaults to $HOME.
	CascadeRoot string `mapstructure:"cascade_root"`

	// CacheEnabled controls whether evaluation caching is enabled.
	CacheEnabled bool `mapstructure:"cache_enabled"`

	// LogEnvDiff controls whether to log environment variable changes to stderr.
	// When true (default), prints +VAR/-VAR/~VAR when loading/unloading .envrc files.
	LogEnvDiff bool `mapstructure:"log_env_diff"`
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		WhitelistPrefix: nil,
		BashPath:        "",
		DisabledShells:  nil,
		CascadeRoot:     "",
		CacheEnabled:    true,
		LogEnvDiff:      true,
	}
}

// Load reads configuration from file and environment variables.
// Configuration is loaded from (in order of precedence):
//  1. Environment variables (CASCADE_*)
//  2. Config file ($XDG_CONFIG_HOME/cascade/config.toml or ~/.config/cascade/config.toml)
//  3. Default values
func Load() (*Config, error) {
	v := viper.New()

	// Set defaults for all config keys
	v.SetDefault("whitelist_prefix", []string{})
	v.SetDefault("bash_path", "")
	v.SetDefault("disabled_shells", []string{})
	v.SetDefault("cascade_root", "")
	v.SetDefault("cache_enabled", true)
	v.SetDefault("log_env_diff", true)

	// Config file settings
	v.SetConfigName("config")
	v.SetConfigType("toml")

	// Add config paths in order of precedence
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		v.AddConfigPath(filepath.Join(xdgConfig, "cascade"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "cascade"))
	}

	// Environment variable overrides
	v.SetEnvPrefix("CASCADE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file (ignore error if file doesn't exist)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// Only return error if it's not a "file not found" error
			return nil, err
		}
	}

	cfg := Default()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ConfigFile returns the path to the config file that was loaded, or empty if none.
func ConfigFile() string {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("toml")

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		v.AddConfigPath(filepath.Join(xdgConfig, "cascade"))
	}

	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "cascade"))
	}

	if err := v.ReadInConfig(); err != nil {
		return ""
	}

	return v.ConfigFileUsed()
}

// IsWhitelisted checks if a path is under any whitelisted prefix.
// Returns true if the path starts with any prefix in WhitelistPrefix.
func (c *Config) IsWhitelisted(path string) bool {
	if c == nil || len(c.WhitelistPrefix) == 0 {
		return false
	}

	// Clean the path for consistent comparison
	cleanPath := filepath.Clean(path)

	for _, prefix := range c.WhitelistPrefix {
		cleanPrefix := filepath.Clean(prefix)
		if cleanPrefix == "" {
			continue
		}

		// Check if path is under prefix
		// We need to ensure it's a proper prefix (directory boundary)
		if strings.HasPrefix(cleanPath, cleanPrefix) {
			// Ensure we're at a directory boundary
			rest := cleanPath[len(cleanPrefix):]
			if rest == "" || rest[0] == filepath.Separator {
				return true
			}
		}
	}

	return false
}

// IsShellDisabled checks if a shell is in the disabled list.
func (c *Config) IsShellDisabled(shell string) bool {
	if c == nil {
		return false
	}

	for _, disabled := range c.DisabledShells {
		if strings.EqualFold(disabled, shell) {
			return true
		}
	}

	return false
}

// GetCascadeRoot returns the cascade root directory.
// Returns CascadeRoot if set, otherwise returns the user's home directory.
func (c *Config) GetCascadeRoot() (string, error) {
	if c != nil && c.CascadeRoot != "" {
		return c.CascadeRoot, nil
	}
	return os.UserHomeDir()
}

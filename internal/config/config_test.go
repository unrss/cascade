package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	t.Parallel()

	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	if !cfg.CacheEnabled {
		t.Error("CacheEnabled should default to true")
	}

	if len(cfg.WhitelistPrefix) != 0 {
		t.Errorf("WhitelistPrefix should be empty, got %v", cfg.WhitelistPrefix)
	}

	if cfg.BashPath != "" {
		t.Errorf("BashPath should be empty, got %q", cfg.BashPath)
	}
}

func TestIsWhitelisted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prefixes   []string
		path       string
		wantResult bool
	}{
		{
			name:       "nil config",
			prefixes:   nil,
			path:       "/home/user/project",
			wantResult: false,
		},
		{
			name:       "empty prefixes",
			prefixes:   []string{},
			path:       "/home/user/project",
			wantResult: false,
		},
		{
			name:       "exact match",
			prefixes:   []string{"/home/user/trusted"},
			path:       "/home/user/trusted",
			wantResult: true,
		},
		{
			name:       "path under prefix",
			prefixes:   []string{"/home/user/trusted"},
			path:       "/home/user/trusted/project",
			wantResult: true,
		},
		{
			name:       "path under prefix deep",
			prefixes:   []string{"/home/user/trusted"},
			path:       "/home/user/trusted/a/b/c/project",
			wantResult: true,
		},
		{
			name:       "path not under prefix",
			prefixes:   []string{"/home/user/trusted"},
			path:       "/home/user/untrusted/project",
			wantResult: false,
		},
		{
			name:       "partial prefix match not allowed",
			prefixes:   []string{"/home/user/trust"},
			path:       "/home/user/trusted/project",
			wantResult: false,
		},
		{
			name:       "multiple prefixes first matches",
			prefixes:   []string{"/home/user/trusted", "/opt/company"},
			path:       "/home/user/trusted/project",
			wantResult: true,
		},
		{
			name:       "multiple prefixes second matches",
			prefixes:   []string{"/home/user/trusted", "/opt/company"},
			path:       "/opt/company/project",
			wantResult: true,
		},
		{
			name:       "multiple prefixes none match",
			prefixes:   []string{"/home/user/trusted", "/opt/company"},
			path:       "/tmp/project",
			wantResult: false,
		},
		{
			name:       "trailing slash in prefix",
			prefixes:   []string{"/home/user/trusted/"},
			path:       "/home/user/trusted/project",
			wantResult: true,
		},
		{
			name:       "empty prefix ignored",
			prefixes:   []string{"", "/home/user/trusted"},
			path:       "/home/user/trusted/project",
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			if tt.prefixes != nil {
				cfg = &Config{WhitelistPrefix: tt.prefixes}
			}

			got := cfg.IsWhitelisted(tt.path)
			if got != tt.wantResult {
				t.Errorf("IsWhitelisted(%q) = %v, want %v", tt.path, got, tt.wantResult)
			}
		})
	}
}

func TestIsShellDisabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		disabled []string
		shell    string
		want     bool
	}{
		{
			name:     "nil config",
			disabled: nil,
			shell:    "bash",
			want:     false,
		},
		{
			name:     "empty disabled list",
			disabled: []string{},
			shell:    "bash",
			want:     false,
		},
		{
			name:     "shell disabled",
			disabled: []string{"fish"},
			shell:    "fish",
			want:     true,
		},
		{
			name:     "shell not disabled",
			disabled: []string{"fish"},
			shell:    "bash",
			want:     false,
		},
		{
			name:     "case insensitive",
			disabled: []string{"FISH"},
			shell:    "fish",
			want:     true,
		},
		{
			name:     "multiple disabled",
			disabled: []string{"fish", "zsh"},
			shell:    "zsh",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var cfg *Config
			if tt.disabled != nil {
				cfg = &Config{DisabledShells: tt.disabled}
			}

			got := cfg.IsShellDisabled(tt.shell)
			if got != tt.want {
				t.Errorf("IsShellDisabled(%q) = %v, want %v", tt.shell, got, tt.want)
			}
		})
	}
}

func TestGetCascadeRoot(t *testing.T) {
	t.Parallel()

	t.Run("custom root", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{CascadeRoot: "/custom/root"}
		got, err := cfg.GetCascadeRoot()
		if err != nil {
			t.Fatalf("GetCascadeRoot() error = %v", err)
		}
		if got != "/custom/root" {
			t.Errorf("GetCascadeRoot() = %q, want %q", got, "/custom/root")
		}
	})

	t.Run("default to home", func(t *testing.T) {
		t.Parallel()

		cfg := &Config{}
		got, err := cfg.GetCascadeRoot()
		if err != nil {
			t.Fatalf("GetCascadeRoot() error = %v", err)
		}

		home, _ := os.UserHomeDir()
		if got != home {
			t.Errorf("GetCascadeRoot() = %q, want %q", got, home)
		}
	})

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		var cfg *Config
		got, err := cfg.GetCascadeRoot()
		if err != nil {
			t.Fatalf("GetCascadeRoot() error = %v", err)
		}

		home, _ := os.UserHomeDir()
		if got != home {
			t.Errorf("GetCascadeRoot() = %q, want %q", got, home)
		}
	})
}

func TestLoad_WithConfigFile(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Create temp config directory
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "cascade")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write config file
	configContent := `
whitelist_prefix = ["/home/user/trusted", "/opt/company"]
bash_path = "/usr/local/bin/bash"
disabled_shells = ["fish"]
cascade_root = "/home/user"
cache_enabled = false
`
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Set HOME to temp dir
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify loaded values
	if len(cfg.WhitelistPrefix) != 2 {
		t.Errorf("WhitelistPrefix = %v, want 2 items", cfg.WhitelistPrefix)
	}

	if cfg.BashPath != "/usr/local/bin/bash" {
		t.Errorf("BashPath = %q, want %q", cfg.BashPath, "/usr/local/bin/bash")
	}

	if len(cfg.DisabledShells) != 1 || cfg.DisabledShells[0] != "fish" {
		t.Errorf("DisabledShells = %v, want [fish]", cfg.DisabledShells)
	}

	if cfg.CascadeRoot != "/home/user" {
		t.Errorf("CascadeRoot = %q, want %q", cfg.CascadeRoot, "/home/user")
	}

	if cfg.CacheEnabled {
		t.Error("CacheEnabled = true, want false")
	}
}

func TestLoad_NoConfigFile(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Use temp dir with no config file
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should get defaults
	if !cfg.CacheEnabled {
		t.Error("CacheEnabled should default to true")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()

	// Use temp dir with no config file
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("CASCADE_CACHE_ENABLED", "false")
	t.Setenv("CASCADE_BASH_PATH", "/custom/bash")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.CacheEnabled {
		t.Error("CacheEnabled should be false from env")
	}

	if cfg.BashPath != "/custom/bash" {
		t.Errorf("BashPath = %q, want %q", cfg.BashPath, "/custom/bash")
	}
}

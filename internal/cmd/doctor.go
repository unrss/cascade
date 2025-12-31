package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unrss/cascade/internal/config"
	"github.com/unrss/cascade/internal/shell"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check cascade installation for common issues",
		Long: `Run diagnostic checks to identify potential issues with your cascade setup.

Checks performed:
  - Shell hook installation (bash, zsh, fish)
  - Bash version compatibility (requires 4.0+)
  - XDG data directory permissions
  - Configuration file validity
  - Cache directory state
  - Common misconfigurations`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

type checkResult struct {
	name    string
	status  string // "ok", "warn", "error", "skip"
	message string
	detail  string // optional additional info
}

func runDoctor(stdout, stderr io.Writer) error {
	c := newColorizer(stdout)

	fmt.Fprintf(stdout, "%s\n\n", c.bold("Cascade Doctor"))

	var results []checkResult

	// Run all checks
	results = append(results, checkBashVersion(c))
	results = append(results, checkDataDirectory(c))
	results = append(results, checkConfigFile(c))
	results = append(results, checkCacheDirectory(c))
	results = append(results, checkShellHooks(c)...)
	results = append(results, checkCascadeRoot(c))

	// Output results
	var warnings, errors int
	for _, r := range results {
		var icon string
		switch r.status {
		case "ok":
			icon = c.green("✓")
		case "warn":
			icon = c.yellow("!")
			warnings++
		case "error":
			icon = c.red("✗")
			errors++
		case "skip":
			icon = c.dim("○")
		}

		fmt.Fprintf(stdout, "  %s %s: %s\n", icon, r.name, r.message)
		if r.detail != "" {
			for _, line := range strings.Split(r.detail, "\n") {
				fmt.Fprintf(stdout, "      %s\n", c.dim(line))
			}
		}
	}

	fmt.Fprintln(stdout)

	// Summary
	if errors > 0 {
		fmt.Fprintf(stdout, "%s Found %d error(s) and %d warning(s)\n", c.red("✗"), errors, warnings)
		return fmt.Errorf("doctor found %d error(s)", errors)
	} else if warnings > 0 {
		fmt.Fprintf(stdout, "%s Found %d warning(s), but cascade should work\n", c.yellow("!"), warnings)
	} else {
		fmt.Fprintf(stdout, "%s All checks passed\n", c.green("✓"))
	}

	return nil
}

func checkBashVersion(c *colorizer) checkResult {
	result := checkResult{name: "Bash version"}

	// Find bash binary
	bashPath := cfg.BashPath
	if bashPath == "" {
		var err error
		bashPath, err = exec.LookPath("bash")
		if err != nil {
			result.status = "error"
			result.message = "bash not found in PATH"
			return result
		}
	}

	// Get bash version
	out, err := exec.Command(bashPath, "--version").Output()
	if err != nil {
		result.status = "error"
		result.message = fmt.Sprintf("failed to run bash: %v", err)
		return result
	}

	// Parse version from "GNU bash, version X.Y.Z..."
	versionRegex := regexp.MustCompile(`version (\d+)\.(\d+)`)
	matches := versionRegex.FindStringSubmatch(string(out))
	if matches == nil {
		result.status = "warn"
		result.message = "could not parse bash version"
		result.detail = strings.TrimSpace(strings.Split(string(out), "\n")[0])
		return result
	}

	major := matches[1]
	minor := matches[2]
	version := fmt.Sprintf("%s.%s", major, minor)

	// Check minimum version (4.0 for associative arrays)
	if major < "4" {
		result.status = "error"
		result.message = fmt.Sprintf("bash %s is too old (requires 4.0+)", version)
		result.detail = "Upgrade bash or set bash_path in config to a newer version"
		return result
	}

	result.status = "ok"
	result.message = fmt.Sprintf("bash %s (%s)", version, bashPath)
	return result
}

func checkDataDirectory(c *colorizer) checkResult {
	result := checkResult{name: "Data directory"}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			result.status = "error"
			result.message = "could not determine home directory"
			return result
		}
		dataHome = filepath.Join(home, ".local", "share")
	}

	cascadeDir := filepath.Join(dataHome, "cascade")

	info, err := os.Stat(cascadeDir)
	if os.IsNotExist(err) {
		result.status = "ok"
		result.message = fmt.Sprintf("%s (will be created on first use)", cascadeDir)
		return result
	}
	if err != nil {
		result.status = "error"
		result.message = fmt.Sprintf("cannot access %s: %v", cascadeDir, err)
		return result
	}

	if !info.IsDir() {
		result.status = "error"
		result.message = fmt.Sprintf("%s exists but is not a directory", cascadeDir)
		return result
	}

	// Check permissions (should be user-only writable)
	mode := info.Mode().Perm()
	if runtime.GOOS != "windows" && mode&0022 != 0 {
		result.status = "warn"
		result.message = fmt.Sprintf("%s has permissive permissions (%o)", cascadeDir, mode)
		result.detail = "Consider: chmod 700 " + cascadeDir
		return result
	}

	// Check subdirectories
	subdirs := []string{"allow", "deny", "trust"}
	var existing []string
	for _, subdir := range subdirs {
		if _, err := os.Stat(filepath.Join(cascadeDir, subdir)); err == nil {
			existing = append(existing, subdir)
		}
	}

	result.status = "ok"
	if len(existing) > 0 {
		result.message = fmt.Sprintf("%s (%s)", cascadeDir, strings.Join(existing, ", "))
	} else {
		result.message = cascadeDir
	}
	return result
}

func checkConfigFile(c *colorizer) checkResult {
	result := checkResult{name: "Config file"}

	configFile := config.ConfigFile()
	if configFile == "" {
		result.status = "ok"
		result.message = "no config file (using defaults)"
		return result
	}

	// Config was already loaded successfully if we're here (via PersistentPreRunE)
	result.status = "ok"
	result.message = configFile
	return result
}

func checkCacheDirectory(c *colorizer) checkResult {
	result := checkResult{name: "Cache directory"}

	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			result.status = "error"
			result.message = "could not determine home directory"
			return result
		}
		cacheHome = filepath.Join(home, ".cache")
	}

	cascadeCache := filepath.Join(cacheHome, "cascade")

	if !cfg.CacheEnabled {
		result.status = "ok"
		result.message = "caching disabled"
		return result
	}

	info, err := os.Stat(cascadeCache)
	if os.IsNotExist(err) {
		result.status = "ok"
		result.message = fmt.Sprintf("%s (will be created when needed)", cascadeCache)
		return result
	}
	if err != nil {
		result.status = "warn"
		result.message = fmt.Sprintf("cannot access %s: %v", cascadeCache, err)
		return result
	}

	if !info.IsDir() {
		result.status = "error"
		result.message = fmt.Sprintf("%s exists but is not a directory", cascadeCache)
		return result
	}

	// Count cache entries
	entries, err := os.ReadDir(cascadeCache)
	if err != nil {
		result.status = "warn"
		result.message = fmt.Sprintf("cannot read %s: %v", cascadeCache, err)
		return result
	}

	result.status = "ok"
	result.message = fmt.Sprintf("%s (%d entries)", cascadeCache, len(entries))
	return result
}

func checkShellHooks(c *colorizer) []checkResult {
	var results []checkResult

	currentShell := detectCurrentShell()

	for _, shellName := range shell.Supported() {
		sh := shell.Get(shellName)
		if sh == nil {
			continue
		}

		result := checkResult{name: fmt.Sprintf("Shell hook (%s)", shellName)}

		// Check if this shell is disabled
		if cfg.IsShellDisabled(shellName) {
			result.status = "skip"
			result.message = "disabled in config"
			results = append(results, result)
			continue
		}

		rcPath := getShellRCPath(shellName)
		if rcPath == "" {
			result.status = "skip"
			result.message = "RC file path unknown"
			results = append(results, result)
			continue
		}

		// Check if RC file exists
		content, err := os.ReadFile(rcPath)
		if os.IsNotExist(err) {
			if shellName == currentShell {
				result.status = "warn"
				result.message = fmt.Sprintf("%s does not exist", rcPath)
			} else {
				result.status = "skip"
				result.message = fmt.Sprintf("%s does not exist", rcPath)
			}
			results = append(results, result)
			continue
		}
		if err != nil {
			result.status = "warn"
			result.message = fmt.Sprintf("cannot read %s: %v", rcPath, err)
			results = append(results, result)
			continue
		}

		// Check for cascade hook
		hookPatterns := []string{
			"cascade hook",
			"eval \"$(cascade",
			"cascade hook " + shellName,
		}

		hasHook := false
		for _, pattern := range hookPatterns {
			if strings.Contains(string(content), pattern) {
				hasHook = true
				break
			}
		}

		if hasHook {
			result.status = "ok"
			result.message = fmt.Sprintf("hook found in %s", rcPath)
		} else if shellName == currentShell {
			result.status = "warn"
			result.message = fmt.Sprintf("hook not found in %s", rcPath)
			result.detail = fmt.Sprintf("Add to %s: eval \"$(cascade hook %s)\"", rcPath, shellName)
		} else {
			result.status = "skip"
			result.message = fmt.Sprintf("hook not found in %s (not current shell)", rcPath)
		}

		results = append(results, result)
	}

	return results
}

func checkCascadeRoot(c *colorizer) checkResult {
	result := checkResult{name: "Cascade root"}

	root, err := cfg.GetCascadeRoot()
	if err != nil {
		result.status = "error"
		result.message = fmt.Sprintf("could not determine cascade root: %v", err)
		return result
	}

	info, err := os.Stat(root)
	if err != nil {
		result.status = "error"
		result.message = fmt.Sprintf("cascade root does not exist: %s", root)
		return result
	}

	if !info.IsDir() {
		result.status = "error"
		result.message = fmt.Sprintf("cascade root is not a directory: %s", root)
		return result
	}

	source := "(default: $HOME)"
	if cfg.CascadeRoot != "" {
		source = "(from config)"
	}

	result.status = "ok"
	result.message = fmt.Sprintf("%s %s", root, source)
	return result
}

func detectCurrentShell() string {
	// Try SHELL environment variable
	shellPath := os.Getenv("SHELL")
	if shellPath != "" {
		base := filepath.Base(shellPath)
		if shell.Get(base) != nil {
			return base
		}
	}

	return ""
}

func getShellRCPath(shellName string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	switch shellName {
	case "bash":
		// Check .bashrc first, then .bash_profile
		bashrc := filepath.Join(home, ".bashrc")
		if _, err := os.Stat(bashrc); err == nil {
			return bashrc
		}
		return filepath.Join(home, ".bash_profile")
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return ""
	}
}

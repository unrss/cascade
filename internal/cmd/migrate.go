package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unrss/cascade/internal/allow"
	"github.com/unrss/cascade/internal/envrc"
)

// incompatiblePattern describes a pattern that may not work in cascade.
type incompatiblePattern struct {
	pattern *regexp.Regexp
	warning string
}

var incompatiblePatterns = []incompatiblePattern{
	{regexp.MustCompile(`\buse_nix\b`), "use_nix is not supported - consider using nix-direnv or mise"},
	{regexp.MustCompile(`\buse_flake\b`), "use_flake is not supported - consider using nix-direnv"},
	{regexp.MustCompile(`\blayout\s+python`), "layout python may work differently - test after migration"},
	{regexp.MustCompile(`\blayout\s+ruby`), "layout ruby may work differently - test after migration"},
	{regexp.MustCompile(`\blayout\s+node`), "layout node may work differently - test after migration"},
	{regexp.MustCompile(`\bsource_up\b`), "source_up is handled automatically by cascade - remove this line"},
	{regexp.MustCompile(`\bDIRENV_`), "DIRENV_* variables should be changed to CASCADE_*"},
}

// migrationResult holds the outcome of migrating a single file.
type migrationResult struct {
	path     string
	migrated bool
	skipped  bool
	reason   string
}

// compatibilityWarning holds a warning about an incompatible pattern.
type compatibilityWarning struct {
	path    string
	line    int
	pattern string
	warning string
}

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate from direnv to cascade",
		Long: `Imports your direnv allow list and checks for compatibility.

This command will:
1. Import allowed .envrc files from direnv
2. Warn about .envrc patterns that may not work in cascade
3. Generate a migration report`,
		RunE: runMigrate,
	}

	cmd.Flags().Bool("dry-run", false, "Show what would be migrated without making changes")
	cmd.Flags().Bool("check-only", false, "Only check for compatibility issues")

	return cmd
}

func runMigrate(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	checkOnly, _ := cmd.Flags().GetBool("check-only")

	out := cmd.OutOrStdout()

	// Find direnv data directory
	direnvDataDir := findDirenvDataDir()
	if direnvDataDir == "" {
		return fmt.Errorf("direnv data directory not found (checked $XDG_DATA_HOME/direnv and ~/.local/share/direnv)")
	}

	fmt.Fprintln(out, "Cascade Migration Report")
	fmt.Fprintln(out, "========================")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Direnv data directory: %s\n", direnvDataDir)
	fmt.Fprintln(out)

	// Read allowed files from direnv
	allowedPaths, err := readDirenvAllowList(direnvDataDir)
	if err != nil {
		return fmt.Errorf("read direnv allow list: %w", err)
	}

	if len(allowedPaths) == 0 {
		fmt.Fprintln(out, "No allowed files found in direnv.")
		return nil
	}

	fmt.Fprintf(out, "Allowed files found: %d\n", len(allowedPaths))

	// Create cascade allow store (unless check-only)
	var store *allow.Store
	if !checkOnly {
		store, err = allow.NewStore()
		if err != nil {
			return fmt.Errorf("create allow store: %w", err)
		}
	}

	// Process each allowed file
	var results []migrationResult
	var warnings []compatibilityWarning

	for _, path := range allowedPaths {
		result := migrationResult{path: path}

		// Check if file exists
		rc, err := envrc.NewRC(path)
		if err != nil {
			result.skipped = true
			result.reason = fmt.Sprintf("error: %v", err)
			results = append(results, result)
			continue
		}

		if !rc.Exists {
			result.skipped = true
			result.reason = "file not found"
			results = append(results, result)
			continue
		}

		// Check for compatibility issues
		fileWarnings := checkCompatibility(path)
		warnings = append(warnings, fileWarnings...)

		// Migrate (allow in cascade) unless dry-run or check-only
		if !checkOnly && !dryRun {
			if err := store.Allow(rc); err != nil {
				result.skipped = true
				result.reason = fmt.Sprintf("allow failed: %v", err)
				results = append(results, result)
				continue
			}
		}

		result.migrated = true
		results = append(results, result)
	}

	// Print results
	printMigrationResults(out, results, dryRun, checkOnly)

	// Print compatibility warnings
	if len(warnings) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Compatibility warnings:")
		printCompatibilityWarnings(out, warnings)
	}

	// Print summary
	printMigrationSummary(out, results, warnings, dryRun, checkOnly)

	return nil
}

// findDirenvDataDir locates the direnv data directory.
func findDirenvDataDir() string {
	// Check XDG_DATA_HOME first
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		direnvDir := filepath.Join(dataHome, "direnv")
		if isDir(direnvDir) {
			return direnvDir
		}
	}

	// Fall back to ~/.local/share/direnv
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	direnvDir := filepath.Join(home, ".local", "share", "direnv")
	if isDir(direnvDir) {
		return direnvDir
	}

	return ""
}

// isDir returns true if path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// readDirenvAllowList reads all allowed file paths from direnv's allow directory.
func readDirenvAllowList(direnvDataDir string) ([]string, error) {
	allowDir := filepath.Join(direnvDataDir, "allow")

	entries, err := os.ReadDir(allowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read allow directory: %w", err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Each allow file contains the path to the allowed .envrc
		allowFile := filepath.Join(allowDir, entry.Name())
		content, err := os.ReadFile(allowFile)
		if err != nil {
			continue // Skip files we can't read
		}

		// The content is the path to the .envrc file
		path := strings.TrimSpace(string(content))
		if path != "" {
			paths = append(paths, path)
		}
	}

	return paths, nil
}

// checkCompatibility scans an .envrc file for incompatible patterns.
func checkCompatibility(path string) []compatibilityWarning {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var warnings []compatibilityWarning
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		for _, p := range incompatiblePatterns {
			if p.pattern.MatchString(line) {
				warnings = append(warnings, compatibilityWarning{
					path:    path,
					line:    lineNum,
					pattern: p.pattern.String(),
					warning: p.warning,
				})
			}
		}
	}

	return warnings
}

// printMigrationResults prints the per-file migration results.
func printMigrationResults(out io.Writer, results []migrationResult, dryRun, checkOnly bool) {
	for _, r := range results {
		if r.migrated {
			action := "migrated"
			if dryRun {
				action = "would migrate"
			} else if checkOnly {
				action = "found"
			}
			fmt.Fprintf(out, "  ✓ %s (%s)\n", r.path, action)
		} else if r.skipped {
			fmt.Fprintf(out, "  ⚠ %s (%s - skipped)\n", r.path, r.reason)
		}
	}
}

// printCompatibilityWarnings prints grouped compatibility warnings.
func printCompatibilityWarnings(out io.Writer, warnings []compatibilityWarning) {
	// Group warnings by file
	byFile := make(map[string][]compatibilityWarning)
	var fileOrder []string

	for _, w := range warnings {
		if _, seen := byFile[w.path]; !seen {
			fileOrder = append(fileOrder, w.path)
		}
		byFile[w.path] = append(byFile[w.path], w)
	}

	for _, path := range fileOrder {
		fmt.Fprintf(out, "  %s:\n", path)
		for _, w := range byFile[path] {
			fmt.Fprintf(out, "    Line %d: %s\n", w.line, w.warning)
		}
	}
}

// printMigrationSummary prints the final summary and next steps.
func printMigrationSummary(out io.Writer, results []migrationResult, warnings []compatibilityWarning, dryRun, checkOnly bool) {
	var migrated, skipped int
	for _, r := range results {
		if r.migrated {
			migrated++
		} else if r.skipped {
			skipped++
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "Summary:")

	if dryRun {
		fmt.Fprintf(out, "  Would migrate: %d files\n", migrated)
	} else if checkOnly {
		fmt.Fprintf(out, "  Found: %d files\n", migrated)
	} else {
		fmt.Fprintf(out, "  Migrated: %d files\n", migrated)
	}

	if skipped > 0 {
		fmt.Fprintf(out, "  Skipped: %d files (not found or errors)\n", skipped)
	}

	if len(warnings) > 0 {
		fmt.Fprintf(out, "  Warnings: %d compatibility issues\n", len(warnings))
	}

	// Print next steps only if we actually migrated something
	if !checkOnly && !dryRun && migrated > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Next steps:")
		fmt.Fprintln(out, "1. Add 'eval \"$(cascade hook bash)\"' to your ~/.bashrc")
		fmt.Fprintln(out, "2. Remove 'eval \"$(direnv hook bash)\"' from your ~/.bashrc")
		if len(warnings) > 0 {
			fmt.Fprintln(out, "3. Review and fix compatibility warnings above")
		}
	}
}

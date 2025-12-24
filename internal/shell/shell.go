// Package shell provides shell-specific formatters for environment export.
package shell

// ShellExport represents environment changes to apply.
// Key present with non-nil value = set variable.
// Key present with nil value = unset variable.
type ShellExport map[string]*string

// Set marks a variable to be set to the given value.
func (e ShellExport) Set(key, value string) {
	e[key] = &value
}

// Unset marks a variable to be unset.
func (e ShellExport) Unset(key string) {
	e[key] = nil
}

// Shell defines the interface for shell-specific output.
type Shell interface {
	// Name returns the shell name (bash, zsh, fish).
	Name() string

	// Hook returns the shell hook code to be eval'd in shell config.
	// selfPath is the path to the cascade binary.
	Hook(selfPath string) string

	// Export formats environment changes as shell commands.
	Export(e ShellExport) string

	// Dump formats a complete environment as shell commands.
	Dump(env map[string]string) string
}

// shells is the registry of supported shell implementations.
var shells = map[string]Shell{
	"bash": Bash,
	"fish": Fish,
	"zsh":  Zsh,
}

// Get returns the Shell implementation for the given name.
// Returns nil if shell is not supported.
func Get(name string) Shell {
	return shells[name]
}

// Supported returns list of supported shell names.
func Supported() []string {
	names := make([]string, 0, len(shells))
	for name := range shells {
		names = append(names, name)
	}
	return names
}

package cmd

import (
	"io"
	"os"

	"golang.org/x/term"
)

// colorizer handles terminal color output.
type colorizer struct {
	enabled bool
}

// newColorizer creates a colorizer that detects terminal capability.
// Colors are disabled if output is not a terminal or NO_COLOR is set.
func newColorizer(w io.Writer) *colorizer {
	enabled := false
	if f, ok := w.(*os.File); ok {
		enabled = term.IsTerminal(int(f.Fd())) && os.Getenv("NO_COLOR") == ""
	}
	return &colorizer{enabled: enabled}
}

func (c *colorizer) green(s string) string {
	if c.enabled {
		return "\033[32m" + s + "\033[0m"
	}
	return s
}

func (c *colorizer) red(s string) string {
	if c.enabled {
		return "\033[31m" + s + "\033[0m"
	}
	return s
}

func (c *colorizer) yellow(s string) string {
	if c.enabled {
		return "\033[33m" + s + "\033[0m"
	}
	return s
}

func (c *colorizer) bold(s string) string {
	if c.enabled {
		return "\033[1m" + s + "\033[0m"
	}
	return s
}

func (c *colorizer) dim(s string) string {
	if c.enabled {
		return "\033[2m" + s + "\033[0m"
	}
	return s
}

func (c *colorizer) cyan(s string) string {
	if c.enabled {
		return "\033[36m" + s + "\033[0m"
	}
	return s
}

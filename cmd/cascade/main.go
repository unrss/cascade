// Package main is the entry point for the cascade CLI.
package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/unrss/cascade/internal/cmd"
)

//go:embed stdlib.sh
var stdlib string

//go:embed version.txt
var version string

func main() {
	if err := cmd.Execute(cmd.Assets{
		Stdlib:  stdlib,
		Version: version,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

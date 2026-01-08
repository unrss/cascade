# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Cascade?

Cascade is a direnv-like environment variable manager with hierarchical inheritance. Unlike direnv (which only loads the nearest `.envrc`), Cascade evaluates the **entire chain** of `.envrc` files from a configured root (default: `$HOME`) down to the current working directory, enabling layered environment configuration.

## Development Commands

```bash
# Build
go build ./cmd/cascade

# Test
go test -race ./...              # Full test suite with race detector
go test -race -short ./...       # Short tests (used in pre-commit)
go test -v ./internal/cmd/...    # Single package with verbose output
go test -run TestExport ./...    # Single test by name

# Lint
golangci-lint run                # Full lint (26 linters configured)
golangci-lint run --fix          # Auto-fix issues

# Security
govulncheck ./...

# Pre-commit (runs all checks)
pre-commit run --all-files
```

## Architecture

### Core Packages

**`internal/envrc/`** - File discovery
- `RC` struct represents a single `.envrc` file with path and SHA256 content hash
- `FindChain()` discovers all `.envrc` files from cascade root to cwd

**`internal/eval/`** - Evaluation engine
- Spawns bash subprocess with embedded `stdlib.sh`
- Uses 3-fd architecture: fd 1→stderr for user output, fd 3 for JSON env dump
- Sets `CASCADE_BIN`, `CASCADE_DIR`, `CASCADE_STDLIB` in subprocess
- `Cache` provides content-hash-based evaluation caching

**`internal/allow/`** - Three-tier authorization
- Allow: by SHA256 content hash (re-allow required if file changes)
- Deny: by path (takes precedence)
- Trust: entire directory subtree auto-allowed

**`internal/shell/`** - Shell-specific exporters (bash/zsh/fish)
- All implement `Shell` interface with `Hook()`, `Export()`, `Dump()` methods

**`internal/state/`** - Persistent state
- Stores applied environment diffs in `~/.local/share/cascade/state/`
- Atomic writes via temp-file-then-rename

**`internal/cmd/`** - Command implementations
- Integration tests in `integration_test.go` build binary once with `sync.Once`

### Key Files

- `stdlib.sh` - Embedded bash standard library (direnv-compatible functions)
- `cmd/cascade/main.go` - Entry point, embeds `stdlib.sh` and `version.txt`

### Data Flow

1. Shell hook calls `cascade export <shell>` on directory change
2. `envrc.FindChain()` discovers `.envrc` files from root to cwd
3. `allow.Store` checks authorization for each file
4. `eval.Evaluator` executes authorized files in order (root first)
5. `shell.Exporter` formats environment changes for the target shell
6. `state.Store` persists applied diff for reversal on directory exit

## Testing Patterns

- Integration tests build the binary once and reuse it (`internal/cmd/integration_test.go`)
- Use `t.TempDir()` for isolated test environments
- Resolve symlinks in tests (macOS `/var` → `/private/var` issues)
- Table-driven tests are preferred

## Import Organization

Imports are grouped (enforced by gci):
1. Standard library
2. External dependencies
3. Internal packages (`github.com/unrss/cascade/...`)

## Linting Notes

- 26 linters enabled via golangci-lint (see `.golangci.yml`)
- Uses `usetesting` linter - use `t.TempDir()`, `t.Setenv()`, `context.WithCancel(t)`
- Blocked packages: `math/rand` (use v2), `golang.org/x/exp` (use stdlib)
- Use `//nolint:gosec` with justification for intentional security bypasses

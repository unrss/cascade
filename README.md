# Cascade

A direnv-like environment variable manager with hierarchical inheritance.

Unlike direnv, which only loads the nearest `.envrc` file, Cascade evaluates the **entire chain** of `.envrc` files from your home directory down to your current working directory. This enables layered environment configuration—global defaults at `~/.envrc`, project-wide settings in `~/projects/.envrc`, and project-specific overrides in `~/projects/myapp/.envrc`.

## Features

- **Hierarchical evaluation**: Discovers and evaluates all `.envrc` files from cascade root to cwd, in order
- **Three-tier security model**: Allow (by content hash), Deny (by path), and Trust (entire subtrees)
- **Tree visualization**: See the full `.envrc` chain and track how variables change at each level
- **Standard library**: `PATH_add`, `layout python|node|go|ruby`, `source_env`, and more
- **Multi-shell support**: bash, zsh, fish
- **direnv migration**: Import existing direnv allow lists

## Installation

### Homebrew

```bash
brew install unrss/tap/cascade
```

### From source

```bash
go install github.com/unrss/cascade/cmd/cascade@latest
```

## Quick Start

1. Add the hook to your shell configuration:

```bash
# bash (~/.bashrc)
eval "$(cascade hook bash)"

# zsh (~/.zshrc)
eval "$(cascade hook zsh)"

# fish (~/.config/fish/config.fish)
cascade hook fish | source
```

2. Create a `.envrc` file:

```bash
echo 'export PROJECT_ENV=development' > .envrc
```

3. Allow it:

```bash
cascade allow
```

4. The environment loads automatically when you enter the directory.

## Commands

| Command | Description |
|---------|-------------|
| `hook <shell>` | Print shell integration hook |
| `allow [path]` | Allow an `.envrc` file (re-allow required if content changes) |
| `deny <path>` | Block an `.envrc` file by path |
| `trust <dir>` | Trust all `.envrc` files under a directory |
| `status` | Show authorization status of discovered `.envrc` files |
| `tree [VAR...]` | Visualize the `.envrc` hierarchy and variable changes |
| `which` | Show which `.envrc` is currently active |
| `dump` | Output the final evaluated environment |
| `migrate` | Import direnv allow list |

### Tree visualization

The `tree` command shows the full chain of `.envrc` files:

```bash
$ cascade tree
~/.envrc [allowed]
  +EDITOR=nvim
~/projects/.envrc [allowed]
  +GOPATH=~/projects/go
~/projects/myapp/.envrc [allowed]
  +DATABASE_URL=postgres://...
```

Track specific variables across the chain:

```bash
$ cascade tree PATH --values
```

## Security Model

Cascade requires explicit authorization before evaluating any `.envrc` file:

- **Allow**: Approves a specific file by its content hash (SHA256). If the file changes, you must re-allow it.
- **Deny**: Blocks a file by path. Takes precedence over allow and trust.
- **Trust**: Marks an entire directory subtree as trusted. All `.envrc` files under that path are auto-allowed.

Authorization data is stored in `~/.local/share/cascade/`.

## Standard Library

Cascade provides bash functions compatible with direnv:

```bash
# Path manipulation
PATH_add bin              # Prepend ./bin to PATH
path_add PATH bin         # Same as PATH_add
MANPATH_add man           # Prepend to MANPATH

# Project layouts
layout python             # Activate/create Python venv
layout node               # Add node_modules/.bin to PATH
layout go                 # Set GOPATH to pwd
layout ruby               # Add .bundle/bin to PATH

# Sourcing
source_env ../.envrc      # Source another .envrc (with auth check)
source_env_if_exists ...  # Source if file exists

# Watching
watch_file .tool-versions # Re-evaluate when file changes
watch_dir config/         # Re-evaluate when directory changes

# Functions
export_function my_func   # Export function to subshells
```

## Configuration

Configuration file: `~/.config/cascade/config.toml`

```toml
# Trust these directory prefixes automatically
whitelist_prefix = ["/home/user/trusted-vendor"]

# Override cascade root (default: $HOME)
cascade_root = "/home/user"

# Path to bash binary
bash_path = "/usr/local/bin/bash"

# Log environment changes to stderr
log_env_diff = true
```

Environment variables override config file settings with the `CASCADE_` prefix:

```bash
export CASCADE_ROOT=/home/user/projects
export CASCADE_LOG_ENV_DIFF=false
```

## Migrating from direnv

Import your existing direnv allow list:

```bash
# Preview what would be imported
cascade migrate --dry-run

# Import
cascade migrate
```

## License

MIT

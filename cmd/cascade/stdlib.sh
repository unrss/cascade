#!/usr/bin/env bash
# cascade stdlib.sh - Standard library for cascade .envrc files
#
# =============================================================================
# FILE DESCRIPTOR STRATEGY
# =============================================================================
#
# Cascade uses a three-fd architecture to separate concerns:
#
#   fd 0 (stdin)  - Standard input (unchanged)
#   fd 1 (stdout) - Redirected to stderr for user-visible output
#   fd 2 (stderr) - Standard error (unchanged, user-visible)
#   fd 3          - Reserved for JSON environment dump (Go reads this)
#
# Why this design?
#
# 1. User scripts can use echo/printf normally - output goes to terminal
# 2. The JSON env dump is isolated on fd 3, never mixed with user output
# 3. Go spawns bash, reads fd 3 for structured data, fd 1+2 for user feedback
#
# Flow:
#   Go spawns: bash -c 'source stdlib.sh; __main__ /path/to/.envrc' 3>&1 1>&2
#   __main__:  Sets up trap, sources .envrc
#   .envrc:    Runs user code, echo goes to terminal (fd 1 → fd 2 → terminal)
#   EXIT trap: Calls __dump_at_exit, JSON goes to fd 3 → Go's stdin
#
# Environment variables set by Go before spawning:
#   CASCADE_BIN  - Absolute path to the cascade binary
#   CASCADE_DIR  - Directory containing the current .envrc being evaluated
#
# =============================================================================

set -euo pipefail

# -----------------------------------------------------------------------------
# Internal Functions
# -----------------------------------------------------------------------------

# Entry point called by Go. Sets up fd redirection and sources the .envrc.
__main__() {
    local envrc_file="${1:-}"

    if [[ -z "$envrc_file" ]]; then
        log_error "no .envrc file specified"
        exit 1
    fi

    if [[ ! -f "$envrc_file" ]]; then
        log_error "file not found: $envrc_file"
        exit 1
    fi

    # Set CASCADE_DIR to the directory containing this .envrc
    # This is used by path helpers and source_env for relative path resolution
    export CASCADE_DIR
    CASCADE_DIR="$(cd "$(dirname "$envrc_file")" && pwd)"

    # Set up exit trap to dump environment as JSON
    trap __dump_at_exit EXIT

    # Source the .envrc file
    # shellcheck source=/dev/null
    source "$envrc_file"
}

# Exit trap handler. Outputs current environment as JSON to fd 3.
# Preserves the original exit code from the .envrc evaluation.
__dump_at_exit() {
    local ret=$?

    # Remove trap to prevent recursion
    trap - EXIT

    # Dump environment as JSON to fd 3
    # CASCADE_BIN must be set by Go before spawning
    if [[ -n "${CASCADE_BIN:-}" ]]; then
        "$CASCADE_BIN" dump json >&3 2>/dev/null || true
    fi

    exit "$ret"
}

# -----------------------------------------------------------------------------
# Logging Functions
# -----------------------------------------------------------------------------

# Log a status message to stderr (visible to user)
log_status() {
    echo "cascade: $*" >&2
}

# Log an error message to stderr (visible to user)
log_error() {
    echo "cascade: error: $*" >&2
}

# -----------------------------------------------------------------------------
# Path Manipulation Functions
# -----------------------------------------------------------------------------

# Prepend a directory to PATH.
# Usage: PATH_add <dir>
# If <dir> is relative, it's resolved relative to CASCADE_DIR.
# Does nothing if the directory is already in PATH.
PATH_add() {
    local dir="${1:-}"

    if [[ -z "$dir" ]]; then
        log_error "PATH_add: missing directory argument"
        return 1
    fi

    # Resolve relative paths against CASCADE_DIR
    if [[ "$dir" != /* ]]; then
        dir="${CASCADE_DIR:-$PWD}/$dir"
    fi

    # Canonicalize the path (resolve symlinks, remove . and ..)
    if [[ -d "$dir" ]]; then
        dir="$(cd "$dir" && pwd)"
    fi

    # Check if already in PATH (exact match)
    case ":${PATH}:" in
        *:"$dir":*)
            # Already in PATH, do nothing
            return 0
            ;;
    esac

    # Prepend to PATH
    export PATH="$dir${PATH:+:$PATH}"
}

# Append a directory to PATH.
# Usage: path_add <dir>
# If <dir> is relative, it's resolved relative to CASCADE_DIR.
# Does nothing if the directory is already in PATH.
path_add() {
    local dir="${1:-}"

    if [[ -z "$dir" ]]; then
        log_error "path_add: missing directory argument"
        return 1
    fi

    # Resolve relative paths against CASCADE_DIR
    if [[ "$dir" != /* ]]; then
        dir="${CASCADE_DIR:-$PWD}/$dir"
    fi

    # Canonicalize the path (resolve symlinks, remove . and ..)
    if [[ -d "$dir" ]]; then
        dir="$(cd "$dir" && pwd)"
    fi

    # Check if already in PATH (exact match)
    case ":${PATH}:" in
        *:"$dir":*)
            # Already in PATH, do nothing
            return 0
            ;;
    esac

    # Append to PATH
    export PATH="${PATH:+$PATH:}$dir"
}

# -----------------------------------------------------------------------------
# Function Export
# -----------------------------------------------------------------------------

# Export a bash function so it's available in subshells.
# Usage: export_function <function_name>
export_function() {
    local name="${1:-}"

    if [[ -z "$name" ]]; then
        log_error "export_function: missing function name"
        return 1
    fi

    # Check if the function exists
    if ! declare -f "$name" >/dev/null 2>&1; then
        log_error "export_function: function '$name' not defined"
        return 1
    fi

    # Export the function
    # shellcheck disable=SC2163 # We're exporting the function named by $name
    export -f "$name"
}

# -----------------------------------------------------------------------------
# Environment Sourcing
# -----------------------------------------------------------------------------

# Source another .envrc file (typically from a sibling directory).
# Usage: source_env <relative_path>
#
# The path is resolved relative to CASCADE_DIR (the directory containing
# the current .envrc). This is for sibling/cousin directory references;
# cascade automatically handles parent directory inheritance.
#
# The target .envrc must be allowed by cascade before it can be sourced.
source_env() {
    local target="${1:-}"

    if [[ -z "$target" ]]; then
        log_error "source_env: missing path argument"
        return 1
    fi

    # Resolve relative paths against CASCADE_DIR
    local target_dir
    if [[ "$target" != /* ]]; then
        target_dir="${CASCADE_DIR:-$PWD}/$target"
    else
        target_dir="$target"
    fi

    # Canonicalize the path
    if [[ ! -d "$target_dir" ]]; then
        log_error "source_env: directory not found: $target_dir"
        return 1
    fi
    target_dir="$(cd "$target_dir" && pwd)"

    local envrc_file="$target_dir/.envrc"

    if [[ ! -f "$envrc_file" ]]; then
        log_error "source_env: .envrc not found in $target_dir"
        return 1
    fi

    # Check if the target .envrc is allowed
    # CASCADE_BIN must be set by Go before spawning
    if [[ -n "${CASCADE_BIN:-}" ]]; then
        if ! "$CASCADE_BIN" check --silent "$envrc_file"; then
            log_error "source_env: $envrc_file is not allowed (run: cascade allow $envrc_file)"
            return 1
        fi
    fi

    # Update CASCADE_DIR for the sourced file
    local saved_cascade_dir="${CASCADE_DIR:-}"
    export CASCADE_DIR="$target_dir"

    # Source the target .envrc
    # shellcheck source=/dev/null
    source "$envrc_file"

    # Restore CASCADE_DIR
    export CASCADE_DIR="$saved_cascade_dir"
}

# -----------------------------------------------------------------------------
# Additional Helpers (direnv compatibility)
# -----------------------------------------------------------------------------

# Add a directory to an arbitrary colon-separated path variable.
# Usage: MANPATH_add /usr/local/man
#        or more generally: pathprepend MYPATH /some/dir
pathprepend() {
    local varname="${1:-}"
    local dir="${2:-}"

    if [[ -z "$varname" ]] || [[ -z "$dir" ]]; then
        log_error "pathprepend: usage: pathprepend VARNAME dir"
        return 1
    fi

    # Resolve relative paths against CASCADE_DIR
    if [[ "$dir" != /* ]]; then
        dir="${CASCADE_DIR:-$PWD}/$dir"
    fi

    # Canonicalize the path
    if [[ -d "$dir" ]]; then
        dir="$(cd "$dir" && pwd)"
    fi

    local current_value="${!varname:-}"

    # Check if already in the path
    case ":${current_value}:" in
        *:"$dir":*)
            return 0
            ;;
    esac

    # Prepend
    printf -v "$varname" '%s' "$dir${current_value:+:$current_value}"
    # shellcheck disable=SC2163 # We're exporting the variable named by $varname
    export "$varname"
}

# Append to an arbitrary colon-separated path variable.
pathappend() {
    local varname="${1:-}"
    local dir="${2:-}"

    if [[ -z "$varname" ]] || [[ -z "$dir" ]]; then
        log_error "pathappend: usage: pathappend VARNAME dir"
        return 1
    fi

    # Resolve relative paths against CASCADE_DIR
    if [[ "$dir" != /* ]]; then
        dir="${CASCADE_DIR:-$PWD}/$dir"
    fi

    # Canonicalize the path
    if [[ -d "$dir" ]]; then
        dir="$(cd "$dir" && pwd)"
    fi

    local current_value="${!varname:-}"

    # Check if already in the path
    case ":${current_value}:" in
        *:"$dir":*)
            return 0
            ;;
    esac

    # Append
    printf -v "$varname" '%s' "${current_value:+$current_value:}$dir"
    # shellcheck disable=SC2163 # We're exporting the variable named by $varname
    export "$varname"
}

# Convenience wrappers for common path variables
MANPATH_add() { pathprepend MANPATH "$1"; }
INFOPATH_add() { pathprepend INFOPATH "$1"; }

# Set an environment variable from a file's contents.
# Usage: source_env_if_exists .env.local
source_env_if_exists() {
    local file="${1:-}"

    if [[ -z "$file" ]]; then
        return 0
    fi

    # Resolve relative paths against CASCADE_DIR
    if [[ "$file" != /* ]]; then
        file="${CASCADE_DIR:-$PWD}/$file"
    fi

    if [[ -f "$file" ]]; then
        # Security check: only source allowed .envrc files
        if [[ -n "${CASCADE_BIN:-}" ]]; then
            if ! "$CASCADE_BIN" check --silent "$file"; then
                log_error "source_env_if_exists: $file is not allowed (run: cascade allow $file)"
                return 1
            fi
        fi
        # shellcheck source=/dev/null
        source "$file"
    fi
}

# Watch a file for changes (no-op in cascade, but provided for direnv compat)
# In direnv, this triggers reload when the file changes.
# Cascade handles this differently via its own file watching.
watch_file() {
    # No-op - cascade handles file watching internally
    :
}

# Layout helpers for common project types
layout() {
    local type="${1:-}"

    case "$type" in
        python|python3)
            # Activate a Python virtual environment
            local venv_dir="${CASCADE_DIR:-$PWD}/.venv"
            if [[ -d "$venv_dir" ]]; then
                export VIRTUAL_ENV="$venv_dir"
                PATH_add "$venv_dir/bin"
                unset PYTHONHOME
            fi
            ;;
        node)
            # Add node_modules/.bin to PATH
            PATH_add node_modules/.bin
            ;;
        go)
            # Set GOPATH to project directory
            export GOPATH="${CASCADE_DIR:-$PWD}"
            PATH_add bin
            ;;
        ruby)
            # Add .bundle/bin to PATH
            PATH_add .bundle/bin
            ;;
        *)
            log_error "layout: unknown layout type: $type"
            return 1
            ;;
    esac
}

# Release Notes

## v0.2.0 (2025-12-30)

### Overview

This release introduces the `cascade doctor` diagnostic command and comprehensive security documentation. No breaking changes—purely additive improvements to help users validate their installation and understand cascade's security model.

### New Features

**Diagnostic Command: `cascade doctor`**

A new diagnostic tool that validates your cascade installation and identifies common issues before they cause problems:

```bash
cascade doctor
```

Checks performed:
- **Bash version**: Verifies bash 4.0+ is available (required for associative arrays)
- **Data directory**: Validates `~/.local/share/cascade/` exists with appropriate permissions
- **Config file**: Confirms configuration loads without errors
- **Cache directory**: Checks cache state and entry count
- **Shell hooks**: Detects whether hooks are installed in your shell RC files
- **Cascade root**: Verifies the root directory exists and is accessible

Output uses colorized status indicators for quick scanning:
- ✓ (green): Check passed
- ! (yellow): Warning—cascade will work, but something may need attention
- ✗ (red): Error—cascade may not function correctly
- ○ (dim): Skipped—not applicable to your configuration

The command exits non-zero if errors are found, making it suitable for CI/CD validation or automated setup scripts.

### Security Improvements

**SECURITY.md Documentation**

Added comprehensive security documentation covering:

- **Authorization model**: Detailed explanation of the three-tier (allow/deny/trust) model with priority order
- **Known considerations**: Explicit documentation of `source_env_if_exists` bypass behavior, trust subtree scope, and plaintext state storage
- **Responsible disclosure**: Security contact (security@unrss.dev), expected response times, and scope definitions
- **Best practices**: Actionable guidance for secure cascade usage

**Directory Permission Validation**

The `doctor` command now warns if `~/.local/share/cascade/` has overly permissive permissions (group or world writable). This helps catch misconfigurations that could allow other users to tamper with authorization state.

### Operational Notes

- `cascade doctor` is read-only—it inspects state but never modifies it
- The command works without shell hooks installed, making it useful for initial setup validation
- All checks complete quickly (no network calls, no expensive operations)

### Upgrade Notes

No action required. This is a purely additive release. Run `cascade doctor` after upgrading to validate your installation.

## v0.1.0 (2025-12-30)

### Overview

Cascade is a direnv-like tool for managing environment variables with hierarchical inheritance across directories. Unlike direnv, which evaluates only the nearest `.envrc` file, cascade evaluates the entire chain of `.envrc` files from a configurable root (default: `$HOME`) down to your current directory, applying them in order.

This initial release provides a production-ready implementation with a three-tier authorization model, multi-shell support, and comprehensive tooling for visibility into your environment configuration.

### Key Features

**Hierarchical .envrc Inheritance**

Cascade automatically discovers and evaluates all `.envrc` files from your home directory (or configured root) to your current working directory. Parent directories set baseline configuration; child directories extend or override as needed.

```
~/
├── .envrc              # Sets EDITOR, base PATH additions
└── work/
    ├── .envrc          # Adds work-specific tools to PATH
    └── project-api/
        └── .envrc      # Sets PROJECT_NAME, API keys
```

When you `cd ~/work/project-api`, all three files are evaluated in order.

**Three-Tier Authorization Model**

- **allow**: Approve a specific `.envrc` file by its content hash (SHA256). If the file changes, re-approval is required.
- **deny**: Explicitly block a specific `.envrc` file by path. Deny takes precedence over allow.
- **trust**: Mark an entire directory subtree as trusted. All `.envrc` files under that path are auto-allowed.

```bash
cascade allow .                    # Allow current directory's .envrc
cascade deny ~/untrusted/.envrc    # Block a specific file
cascade trust ~/work               # Trust all .envrc files under ~/work
```

**Tree Visualization**

The `cascade tree` command shows your entire `.envrc` chain with authorization status and variable changes at each level:

```bash
cascade tree              # Show chain with status
cascade tree --values     # Include variable values
cascade tree PATH         # Track a specific variable through the chain
cascade tree --json       # Machine-readable output
```

**Standard Library Functions**

Cascade provides a bash standard library compatible with common direnv patterns:

- `PATH_add`, `path_add`: Prepend/append to PATH
- `MANPATH_add`, `INFOPATH_add`: Manage other path variables
- `layout python|node|go|ruby`: Common project layouts
- `source_env`: Source another `.envrc` (must be allowed)
- `watch_file`, `watch_dir`: Trigger re-evaluation on file changes
- `export_function`: Export bash functions to subshells

### Installation

**From GitHub Releases (recommended)**

Download the appropriate binary for your platform from the [releases page](https://github.com/unrss/cascade/releases/tag/v0.1.0).

```bash
# Example for macOS ARM64
curl -LO https://github.com/unrss/cascade/releases/download/v0.1.0/cascade_0.1.0_darwin_arm64.tar.gz
tar xzf cascade_0.1.0_darwin_arm64.tar.gz
sudo mv cascade /usr/local/bin/
```

**Shell Integration**

Add the appropriate hook to your shell configuration:

```bash
# ~/.bashrc
eval "$(cascade hook bash)"

# ~/.zshrc
eval "$(cascade hook zsh)"

# ~/.config/fish/config.fish
cascade hook fish | source
```

### Migration from direnv

Cascade includes a migration command that imports your existing direnv allow list:

```bash
cascade migrate              # Import and check compatibility
cascade migrate --dry-run    # Preview without making changes
cascade migrate --check-only # Only check for compatibility issues
```

**Key Differences from direnv**

| Feature        | direnv                | cascade                            |
| -------------- | --------------------- | ---------------------------------- |
| Inheritance    | Single nearest `.envrc` | Full chain from root to cwd        |
| Deny mechanism | None                  | Explicit deny by path              |
| Subtree trust  | None                  | `cascade trust <dir>`                |
| Visualization  | None                  | `cascade tree`                       |
| `source_up`      | Manual                | Automatic (remove `source_up` calls) |

**Compatibility Notes**

The following direnv features are not supported:

- `use_nix`, `use_flake`: Consider nix-direnv or mise
- `DIRENV_*` variables: Rename to `CASCADE_*`

The `source_up` directive is unnecessary—cascade handles parent inheritance automatically. Remove these lines during migration.

### Security Model

Cascade's authorization model is designed to prevent accidental execution of untrusted shell code.

**Content-Based Allow**

When you run `cascade allow`, the file's SHA256 hash is recorded. If the file content changes, it returns to "not allowed" status and requires re-approval. This prevents silent execution of modified files.

**Path-Based Deny**

Deny is keyed by file path, not content. A denied file remains blocked regardless of content changes. Deny takes precedence over allow and trust.

**Subtree Trust**

`cascade trust <directory>` marks all `.envrc` files under that path as implicitly allowed. Use this for directories you control (e.g., `~/work`). Trust is checked after explicit deny, so you can still block specific files within a trusted subtree.

**Authorization Priority**

1. Denied (path-based) — blocks execution
2. Allowed (content-based) — permits execution
3. Trusted subtree (path-based) — permits execution
4. Whitelisted prefix (config-based) — permits execution
5. Not allowed — blocks execution, prompts user

**Known Security Considerations**

- `source_env_if_exists` sources files without authorization checks. Only use this for files you control (e.g., `.env.local` containing secrets).
- State files in `~/.local/share/cascade/` are stored as plaintext. Protect this directory appropriately.
- Subtree trust applies to all current and future `.envrc` files under the path. Use judiciously.

### Known Limitations

The following features are not included in v0.1.0:

- **Nix integration**: `use_nix` and `use_flake` are not supported
- **Automatic reload on external changes**: Files modified outside the shell require manual `cd .` to reload
- **Remote/network paths**: Only local filesystem paths are supported
- **Windows**: Only Linux and macOS are supported

### Configuration

Configuration is loaded from `~/.config/cascade/config.toml` (or `$XDG_CONFIG_HOME/cascade/config.toml`).

```toml
# Directories where .envrc files are auto-allowed (use with caution)
whitelist_prefix = ["/home/user/trusted-vendor"]

# Override the root directory for chain traversal (default: $HOME)
cascade_root = "/home/user"

# Path to bash binary (default: search $PATH)
bash_path = "/usr/local/bin/bash"

# Disable specific shells
disabled_shells = ["fish"]

# Enable/disable evaluation caching (default: true)
cache_enabled = true

# Log environment variable changes to stderr (default: true)
log_env_diff = true
```

Environment variables override config file settings with the `CASCADE_` prefix:

```bash
export CASCADE_ROOT=/home/user/projects
export CASCADE_CACHE_ENABLED=false
```

### Platform Support

**Operating Systems**
- Linux (amd64, arm64)
- macOS (amd64, arm64)

**Shells**
- bash (4.0+)
- zsh
- fish

### Supply Chain Security

All release artifacts are signed and include Software Bill of Materials (SBOM) for supply chain verification.

**Verifying Signatures**

Releases are signed using Cosign with keyless signing (Sigstore). To verify:

```bash
# Install cosign: https://docs.sigstore.dev/cosign/installation/

# Download the release artifacts
curl -LO https://github.com/unrss/cascade/releases/download/v0.1.0/checksums.txt
curl -LO https://github.com/unrss/cascade/releases/download/v0.1.0/checksums.txt.sig
curl -LO https://github.com/unrss/cascade/releases/download/v0.1.0/checksums.txt.pem

# Verify the signature
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  checksums.txt

# Verify your downloaded archive against the checksums
sha256sum -c checksums.txt --ignore-missing
```

**SBOM**

Each release archive includes an SPDX SBOM file (e.g., `cascade_0.1.0_darwin_arm64.tar.gz.sbom.json`) listing all dependencies.

### Breaking Changes

None. This is the initial release.

### Upgrade Notes

Not applicable for the initial release.

### Contributors

Initial release by the cascade team.

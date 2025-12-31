# Security Policy

## Security Model

Cascade evaluates `.envrc` files as bash scripts, which can execute arbitrary code. To prevent unauthorized code execution, Cascade implements a **three-tier authorization model**:

### Authorization Tiers

| Tier | Scope | Persistence | Use Case |
|------|-------|-------------|----------|
| **Allow** | Single file by content hash | Re-approval required if file changes | Default for most `.envrc` files |
| **Deny** | Single file by path | Blocks file regardless of content | Permanently block untrusted files |
| **Trust** | Entire directory subtree | All `.envrc` files auto-allowed | Trusted vendor directories, personal projects |

### Priority Order

When checking authorization: **Deny > Allow > Trust > Whitelist > Not Allowed**

A denied file is never evaluated, even if it's in a trusted subtree.

### Authorization Storage

Authorization data is stored as plaintext files in `~/.local/share/cascade/`:

```
~/.local/share/cascade/
├── allow/      # SHA256(content) → path mapping
├── deny/       # SHA256(path) → path mapping
└── trust/      # SHA256(dir) → directory path
```

These files are readable by the user only. No secrets are stored—just content hashes and paths.

## Known Security Considerations

### source_env_if_exists Bypasses Authorization

The `source_env_if_exists` function sources files **without** checking cascade authorization. This is intentional—it's designed for simple environment files like `.env.local` that contain only variable assignments.

**Risk**: If an attacker can write to a file that gets sourced via `source_env_if_exists`, they can execute arbitrary code.

**Mitigation**:
- Only use `source_env_if_exists` for files you control
- Use `source_env` (which checks authorization) for `.envrc` files from other directories
- Ensure sourced files have appropriate filesystem permissions

### Trust Subtree Scope

The `cascade trust <dir>` command auto-allows **all** `.envrc` files under that directory, including:
- Files created in the future
- Files in deeply nested subdirectories
- Files created by other users (if they have write access)

**Risk**: If you trust a directory where others can create files (shared directories, vendor deps), they can execute code in your shell.

**Mitigation**:
- Only trust directories you fully control
- Prefer explicit `allow` for individual files in shared contexts
- Review trusted subtrees periodically: `cascade status` shows all authorization sources

### Plaintext State Storage

Authorization state is stored as plaintext files. Anyone with read access to `~/.local/share/cascade/` can see which files you've allowed and which directories you trust.

**Risk**: Leaks information about your project structure and trust decisions.

**Mitigation**: The directory is created with user-only permissions (0755). Ensure your home directory permissions are appropriate.

### Content Hash vs Path Security

- **Allow**: Uses SHA256 of file content. If file changes, re-approval is required.
- **Deny**: Uses hash of file path. Blocks that path regardless of content.
- **Trust**: Uses hash of directory path. Applies to all files under that path.

This means:
- Moving an allowed file to a new path requires re-allowing
- Denying a path doesn't affect copies of that file elsewhere
- Symlinks are resolved before hashing

## Supported Versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report security vulnerabilities by emailing:

**security@unrss.dev**

Include:
1. Description of the vulnerability
2. Steps to reproduce
3. Potential impact
4. Any suggested fixes (optional)

### What to Expect

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 7 days
- **Resolution Timeline**: Depends on severity, typically 30-90 days

We will:
1. Confirm receipt of your report
2. Investigate and validate the issue
3. Develop a fix and coordinate disclosure timing with you
4. Credit you in the security advisory (unless you prefer anonymity)

### Scope

In scope:
- Authorization bypass vulnerabilities
- Code execution outside the cascade security model
- Information disclosure from cascade's internal state
- Privilege escalation via cascade

Out of scope:
- Vulnerabilities in `.envrc` files you've explicitly allowed
- Issues requiring local shell access (cascade runs in your shell by design)
- Denial of service via malformed `.envrc` files
- Social engineering attacks

## Security Best Practices

1. **Review before allowing**: Always inspect `.envrc` content before running `cascade allow`
2. **Prefer allow over trust**: Use `trust` sparingly, only for directories you fully control
3. **Use deny for untrusted paths**: Explicitly deny `.envrc` files in untrusted vendor directories
4. **Keep cascade updated**: Security fixes are released as patch versions
5. **Check status regularly**: Run `cascade status` to review your authorization decisions

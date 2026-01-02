# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0] - 2025-12-30

### Added

- 0cc72c0 Add SECURITY.md and cascade doctor command
- bedc0b6 Add comprehensive README and improve release configuration

### Fixed

- d8c6d56 Fix lint warnings in doctor command

### Documentation

- bedc0b6 Add comprehensive README and improve release configuration

## [0.1.0] - 2025-12-30

### Added

- 8d0167a Add envrc testdata fixtures for CI
- 32ec72f Add GitHub Actions CI/CD infrastructure
- 4673f0f Fix env diff spam by comparing effect not full diff
- b49e8b2 Update dependencies
- 4e1f700 Setup beads
- e6ddb59 Only log env diff on directory or content change
- a129cc0 Fix duplicate env printing in zsh hook
- 54a291a Filter ignored variables from cascade tree output
- 5ba068c Update cascade tree command help text with examples
- b53f98b Add comprehensive tests for cascade tree command
- 15f0207 Add variable filtering to cascade tree command
- 6ca1e9f Add variable tracking to cascade tree command
- 5b32b00 Add cascade tree command for visualizing envrc hierarchy
- 6e1b57d Add environment variable diff logging to export
- a6d635f Extract colorizer into shared file
- 526cad3 Add LogEnvDiff config field for env diff logging
- e2fdf8b Add integration tests for persistent state recovery
- a7df071 Integrate persistent state into export for denied file revert
- 9339ea2 Add persistent state package for env diff recovery
- fe38aca Add integration tests for source_env allow/deny enforcement
- 48d5230 Add check command to verify envrc allow status
- d82d2bd Add cascade CLI with all commands
- 0dc8c66 Add config package for TOML configuration
- debb154 Add shell package with bash, zsh, and fish support
- f54860f Add eval package for bash subprocess execution
- 39be1d3 Add allow package for security authorization system
- 7557792 Add envrc package for RC file model and hierarchy discovery
- b38be13 Add env package for environment diffing and serialization

### Changed

- 192b6cd ci: bump the actions group with 4 updates
- 60b8acd Update go.sum with transitive dependency hashes
- cd8c854 Update linter rules
- 8ea869f Fix lint issues identified by golangci-lint
- 14562e3 Run go mod tidy and fix README newline
- 6bc4ec3 Standardize import ordering with gci
- 32e278d Add golangci-lint v2 and pre-commit configuration
- 9b969b5 Update gitignore for build artifacts

### Fixed

- 837c4c6 Close cascade-czt: env diff spam fix

[0.2.0]: https://github.com/unrss/cascade/releases/tag/v0.2.0
[0.1.0]: https://github.com/unrss/cascade/releases/tag/v0.1.0

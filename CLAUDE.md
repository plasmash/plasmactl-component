# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

plasmactl-component is a Go [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages Plasma platform component versioning, dependencies, and chassis attachments. It registers 9 CLI actions (`component:bump`, `component:sync`, `component:depend`, `component:configure`, `component:attach`, `component:detach`, `component:query`, `component:list`, `component:show`).

## Build, Test, and Lint Commands

```bash
make build        # Build binary to ./bin/launchr (CGO_ENABLED=0)
make test         # Run all tests (uses gotestfmt)
make test-short   # Run short tests only
make lint         # Run golangci-lint with auto-fix
make deps         # Download Go module dependencies
DEBUG=1 make build  # Build with debug symbols
```

Run a single test:
```bash
go test -v -run TestName ./internal/repository/...
```

## Architecture

### Plugin System

The entry point is `plugin.go`, which registers the plugin via `init()` → `launchr.RegisterPlugin()`. The `DiscoverActions()` method returns all 9 actions. Each action is defined by:
1. An embedded YAML file (`actions/<name>/<name>.yaml`) describing CLI args/opts
2. A Go struct in `actions/<name>/` with `Execute()` and `Result()` methods
3. Wiring in `plugin.go` that maps CLI input to the struct and calls `action.NewFnRuntimeWithResult()`

All actions receive a logger and terminal via `getLogger()` helper and return structured JSON results.

### Package Layout

- **`actions/`** — Each subdirectory is a CLI action. The YAML defines args/flags, the Go file implements logic. Actions are: `attach`, `bump`, `configure`, `depend`, `detach`, `list`, `query`, `show`, `sync`.
- **`pkg/component/`** — Public component abstraction: `Component` struct, loading from playbooks/filesystem, attachments, version reading from `meta/plasma.yaml`.
- **`internal/playbook/`** — Ansible playbook YAML manipulation: load, save, add/remove roles under chassis hosts. Supports both simple string and extended map role formats.
- **`internal/repository/`** — Git operations via go-git: `Bumper` creates version bump commits, `GetCommits()` identifies changed files. Has tests covering regular repos and git worktrees.
- **`internal/sync/`** — Version propagation engine: `Inventory` builds dependency graph (semantic + build deps), uses topological sorting for correct propagation order. `FilesCrawler` walks filesystem, `Timeline` tracks version changes.

### Key Data Patterns

- **Component naming**: `{layer}.{kind}.{name}` (e.g., `interaction.applications.dashboards`)
- **Versions**: 13-char git commit hash stored in `meta/plasma.yaml` under `plasma.version`
- **Dependencies**: Semantic deps in `tasks/dependencies.yaml`, build deps in `tasks/main.yaml`
- **Chassis attachments**: Components are roles in layer playbooks (`src/{layer}/{layer}.yaml`) under chassis host entries

### Component Lifecycle

```
Edit component → git commit → component:bump (set version to commit hash) → component:sync (propagate to dependents)
```

### depend Action Modes

- **Show mode** (no operations): queries platform graph for dependency tree
- **Operations mode**: kubectl-style mutations (`dep` = add, `dep-` = remove, `old/new` = replace) on `tasks/dependencies.yaml`

## Linting

Uses golangci-lint v2.5.0 with: dupl, errcheck, goconst, gosec, govet, ineffassign, revive, staticcheck, unused. Formatter: goimports. Always run `make lint` before committing — it applies auto-fixes.

## Key Dependencies

- `launchrctl/launchr` — Plugin framework, action system, logger, terminal
- `launchrctl/compose` — Package composition and merging
- `plasmash/plasmactl-platform` — Platform graph for component queries
- `plasmash/plasmactl-chassis` — Chassis path abstractions
- `go-git/go-git` — Pure Go git operations
- `sosedoff/ansible-vault-go` — Vault encryption for secrets
- `stevenle/topsort` — Topological sorting for dependency propagation

## Local Development Setup

The `go.mod` uses `replace` directives pointing to sibling directories. To build locally, clone these repos as siblings:

```bash
git clone git@github.com:launchrctl/launchr.git ../launchr
git clone git@github.com:plasmash/plasmactl-model.git ../plasmactl-model
git clone git@github.com:plasmash/plasmactl-platform.git ../plasmactl-platform
```

`launchr` must be on the `feat/structured-output` branch (provides `action.NewFnRuntimeWithResult`):

```bash
cd ../launchr && git checkout feat/structured-output
```

These replace directives must be removed or updated for CI/release builds.

## Git Worktree Support

All `git.PlainOpen()` calls use `PlainOpenWithOptions` with `EnableDotGitCommonDir: true` to support git worktrees. This is safe for regular repos. When opening a repository for concurrent use (e.g., in worker goroutines), open it once and pass the `*git.Repository` to workers rather than reopening per worker.

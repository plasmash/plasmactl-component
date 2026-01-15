# plasmactl-component

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages component versioning and dependencies in Plasma platforms.

## Overview

`plasmactl-component` automatically updates the version of platform components that were modified in the last commit. It tracks changes using git history and propagates version updates through the dependency tree.

## Features

- **Automatic Versioning**: Updates component versions based on git commit history
- **Dependency Propagation**: Cascades version updates to dependent components
- **Dependency Management**: Query and manage component dependencies
- **Multi-Repository Support**: Works across domain and package repositories
- **Variable Tracking**: Monitors changes in configuration variables (`group_vars`, `vault.yaml`)
- **Smart Filtering**: Excludes documentation files (README.md, README.svg)

## Commands

### component:bump

Update versions of components modified in the last commit:

```bash
plasmactl component:bump
```

Options:
- `--last`: Only consider changes from the last commit
- `--allow-override`: Allow bump even with uncommitted changes

### component:sync

Propagate version changes to all dependent components:

```bash
plasmactl component:sync
```

**Important**: Always run `plasmactl package:compose` before `component:sync` to ensure accurate dependency resolution.

### component:depend

Query and manage component dependencies:

```bash
# Query dependencies
plasmactl component:depend cognition.skills.bar
plasmactl component:depend cognition.skills.bar --up      # What depends on it
plasmactl component:depend cognition.skills.bar --down    # What it depends on

# Add dependency
plasmactl component:depend cognition.skills.bar cognition.function.foo

# Remove dependency (dash prefix)
plasmactl component:depend cognition.skills.bar -cognition.function.foo

# Replace dependency (slash separator)
plasmactl component:depend cognition.skills.bar old.mrn/new.mrn
```

## How It Works

### Bump Flow

1. Opens the git repository
2. Checks if the latest commit is not already a bump commit
3. Collects changed files (new, updated, deleted) until the previous bump commit
4. Gets the short hash of the last commit
5. Iterates through resources and updates their versions

### Sync Flow (Propagation)

1. **Analyze build directory**: Identify resources and their dependencies
2. **Build timeline**: Determine when each resource/variable was last modified
3. **Create propagation map**: Map version updates chronologically
4. **Update resources**: Apply propagated versions to dependent components

## Component Versioning

Versions are git commit hashes stored in `meta/plasma.yaml`:

```yaml
version: abc123def
```

After propagation, dependent resources get compound versions:

```yaml
version: original_version-propagated_version
```

## Resource Criteria

Resources must:
- Have a `meta/plasma.yaml` file
- Match path pattern: `%platform/%kind/roles/%name/`

Supported component kinds:
- `applications`
- `services`
- `softwares`
- `executors`
- `flows`
- `skills`
- `functions`
- `libraries`
- `entities`

## Workflow Example

```bash
# 1. Make changes to components
vim platform/services/roles/myservice/tasks/main.yaml

# 2. Commit changes
git add -A
git commit -m "feat: update myservice"

# 3. Bump versions
plasmactl component:bump

# 4. Compose the platform
plasmactl package:compose

# 5. Propagate versions to dependencies
plasmactl component:sync
```

## Multi-Repository Workflow

When working with packages:

```bash
# In package repository
vim services/roles/myservice/tasks/main.yaml
git commit -m "feat: update service"
plasmactl component:bump

# In platform repository
# Update plasma-compose.yaml to reference new package version
plasmactl package:compose
plasmactl component:sync
```

## Variable Propagation

Changes to variables in `group_vars` or `vault.yaml` trigger propagation to all dependent resources, even without resource bumps:

```bash
vim group_vars/platform.interaction.observability/vars.yaml
git commit -m "config: update variable"
plasmactl package:compose
plasmactl component:sync  # Propagates variable change to all dependent resources
```

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [Plasma Platform](https://plasma.sh) - Platform documentation

## License

[European Union Public License 1.2 (EUPL-1.2)](LICENSE)

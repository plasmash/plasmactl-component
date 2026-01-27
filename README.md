# plasmactl-component

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages component versioning, dependencies, and chassis attachments in Plasma platforms.

## Overview

`plasmactl-component` provides tools for managing Plasma platform components including automatic versioning, dependency management, configuration, and chassis attachments.

## Features

- **Version Bumping**: Automatically update component versions based on git changes
- **Version Syncing**: Propagate version changes through the dependency tree
- **Dependency Management**: kubectl-style add/remove/replace operations on dependencies
- **Chassis Attachments**: Attach and detach components from chassis sections
- **Configuration**: Manage component variables with vault encryption support

## Commands

### component:bump

Update versions of components modified in the last commit:

```bash
plasmactl component:bump
plasmactl component:bump --dry-run
plasmactl component:bump --last
```

Options:
- `--last`: Only consider changes from the last commit
- `--dry-run`: Preview changes without applying

### component:sync

Propagate version changes to all dependent components:

```bash
plasmactl component:sync
plasmactl component:sync --dry-run
plasmactl component:sync --playbook-filter platform.foundation
```

Options:
- `--dry-run`: Preview changes without applying
- `--allow-override`: Allow sync with uncommitted changes
- `--playbook-filter`: Filter by playbook resource usage
- `--time-depth`: Time depth for change detection

### component:depend

Query and manage component dependencies using kubectl-style operations:

```bash
# Show dependencies (no operations = show mode)
plasmactl component:depend cognition.skills.analyzer
plasmactl component:depend cognition.skills.analyzer --tree
plasmactl component:depend cognition.skills.analyzer --path

# Add dependencies
plasmactl component:depend cognition.skills.analyzer cognition.functions.nlp
plasmactl component:depend cognition.skills.analyzer dep1 dep2 dep3

# Remove dependency (trailing dash)
plasmactl component:depend cognition.skills.analyzer cognition.functions.old-

# Replace dependency (slash separator)
plasmactl component:depend cognition.skills.analyzer old.mrn/new.mrn

# Combined operations
plasmactl component:depend cognition.skills.analyzer newdep olddep- v1/v2
```

Options:
- `-s, --source`: Resources source directory (default: `.plasma/compose/merged`)
- `-p, --path`: Show paths instead of MRNs
- `-t, --tree`: Show dependencies in tree-like output
- `-d, --depth`: Limit recursion lookup depth (default: 99)

### component:attach

Attach a component to a chassis section:

```bash
plasmactl component:attach interaction.applications.dashboards platform.interaction.observability
plasmactl component:attach interaction.applications.connect platform.interaction.interop --source ./src
```

Options:
- `-s, --source`: Source directory containing layer playbooks

This modifies the layer playbook (e.g., `interaction/interaction.yaml`) to add the component role under the specified chassis host.

### component:detach

Detach a component from a chassis section:

```bash
plasmactl component:detach interaction.applications.dashboards platform.interaction.observability
plasmactl component:detach interaction.applications.old platform.interaction.legacy --source ./src
```

Options:
- `-s, --source`: Source directory containing layer playbooks

### component:configure

Configure component variables:

```bash
# List all configuration
plasmactl component:configure --list

# Get a specific value
plasmactl component:configure mykey --get

# Set a value
plasmactl component:configure mykey myvalue

# Set with vault encryption
plasmactl component:configure db_password secretvalue --vault

# Validate configuration
plasmactl component:configure --validate

# Generate configuration
plasmactl component:configure --generate
```

Options:
- `--get`: Get value mode
- `--list`: List all configuration
- `--validate`: Validate configuration
- `--generate`: Generate configuration
- `--at`: Target location
- `--vault`: Use vault encryption
- `--format`: Output format (yaml, json)
- `--strict`: Strict validation mode

## Project Structure

```
plasmactl-component/
├── plugin.go                        # Plugin registration
├── actions/
│   ├── attach/
│   │   ├── attach.yaml              # Action definition
│   │   └── attach.go                # Implementation
│   ├── bump/
│   │   ├── bump.yaml
│   │   └── bump.go
│   ├── configure/
│   │   ├── configure.yaml
│   │   └── configure.go
│   ├── depend/
│   │   ├── depend.yaml
│   │   └── depend.go
│   ├── detach/
│   │   ├── detach.yaml
│   │   └── detach.go
│   └── sync/
│       ├── sync.yaml
│       ├── sync.go
│       └── files_crawler.go
└── internal/
    ├── component/                   # Component operations
    │   └── component.go
    └── playbook/                    # Shared playbook operations
        └── playbook.go              # Load, save, add/remove roles
```

## Component Lifecycle

```
Edit component → git commit → component:bump → component:sync → deploy
                                   ↓                ↓
                            Update version   Update dependents
```

## Workflow Example

```bash
# 1. Make changes to components
vim platform/services/roles/myservice/tasks/main.yaml

# 2. Commit changes
git add -A && git commit -m "feat: update myservice"

# 3. Bump versions
plasmactl component:bump

# 4. Propagate versions to dependencies
plasmactl component:sync

# 5. Compose the platform
plasmactl model:compose

# 6. Manage dependencies
plasmactl component:depend mycomponent newdep olddep-

# 7. Attach to chassis
plasmactl component:attach interaction.applications.new platform.interaction.observability
```

## Related Commands

| Plugin | Command | Purpose |
|--------|---------|---------|
| plasmactl-chassis | `chassis:list` | List available chassis sections |
| plasmactl-chassis | `chassis:show` | Show chassis section details |
| plasmactl-model | `model:compose` | Compose packages after version updates |

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [plasmactl-chassis](https://github.com/plasmash/plasmactl-chassis) - Chassis management
- [Plasma Platform](https://plasma.sh) - Platform documentation

## License

[European Union Public License 1.2 (EUPL-1.2)](LICENSE)

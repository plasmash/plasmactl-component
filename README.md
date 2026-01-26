# plasmactl-component

A [Launchr](https://github.com/launchrctl/launchr) plugin for [Plasmactl](https://github.com/plasmash/plasmactl) that manages component versioning, dependencies, and chassis attachments in Plasma platforms.

## Overview

`plasmactl-component` provides tools for managing Plasma platform components including automatic versioning, dependency management, and chassis attachments.

## Commands

### component:bump

Update versions of components modified in the last commit:

```bash
plasmactl component:bump
```

Options:
- `--last`: Only consider changes from the last commit
- `--dry-run`: Preview changes without applying

### component:sync

Propagate version changes to all dependent components:

```bash
plasmactl component:sync
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
- `-s, --source`: Resources source directory (default: `.plasma/package/compose/merged`)
- `-p, --path`: Show paths instead of MRNs
- `-t, --tree`: Show dependencies in tree-like output
- `-d, --depth`: Limit recursion lookup depth (default: 99)

### component:attach

Attach a component to a chassis section:

```bash
plasmactl component:attach interaction.applications.dashboards platform.interaction.observability
```

Options:
- `-s, --source`: Source directory

### component:detach

Detach a component from a chassis section:

```bash
plasmactl component:detach interaction.applications.dashboards platform.interaction.observability
```

Options:
- `-s, --source`: Source directory

### component:configure

Configure component variables:

```bash
# List all configuration
plasmactl component:configure --list

# Get a specific value
plasmactl component:configure mykey --get

# Set a value
plasmactl component:configure mykey myvalue

# Validate configuration
plasmactl component:configure --validate
```

Options:
- `--get`: Get value mode
- `--list`: List all configuration
- `--validate`: Validate configuration
- `--generate`: Generate configuration
- `--at`: Target location
- `--vault`: Use vault encryption
- `--format`: Output format
- `--strict`: Strict validation mode

## Workflow Example

```bash
# 1. Make changes to components
vim platform/services/roles/myservice/tasks/main.yaml

# 2. Commit changes
git add -A && git commit -m "feat: update myservice"

# 3. Bump versions
plasmactl component:bump

# 4. Compose the platform
plasmactl package:compose

# 5. Propagate versions to dependencies
plasmactl component:sync

# 6. Manage dependencies
plasmactl component:depend mycomponent newdep olddep-
```

## Documentation

- [Plasmactl](https://github.com/plasmash/plasmactl) - Main CLI tool
- [Plasma Platform](https://plasma.sh) - Platform documentation

## License

[European Union Public License 1.2 (EUPL-1.2)](LICENSE)

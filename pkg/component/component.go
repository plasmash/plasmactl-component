// Package component provides types and operations for managing platform components.
// Components are logical units (applications, services, flows, etc.) that attach to chassis paths.
package component

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/plasmash/plasmactl-model/pkg/model"
	"gopkg.in/yaml.v3"
)

// Component represents a platform component discovered from playbooks.
type Component struct {
	Name     string // Full component name (e.g., "interaction.applications.dashboards")
	Kind     string // Component kind (e.g., "applications", "services", "flows")
	Layer    string // Layer (e.g., "interaction", "foundation", "cognition")
	Version  string // Component version (git commit hash from meta/plasma.yaml)
	Playbook string // Path to playbook where component is defined
	Chassis  string // Chassis path where component is attached
}

// FormatVersion returns the version or "-" if empty.
func FormatVersion(version string) string {
	if version == "" {
		return "-"
	}
	return version
}

// FormatDisplayName formats a component name with version (e.g., "name@version" or "name@-" if no version).
func FormatDisplayName(name, version string) string {
	return name + "@" + FormatVersion(version)
}

// DisplayName returns the component name with version (e.g., "interaction.applications.dashboards@abc123").
func (c Component) DisplayName() string {
	return FormatDisplayName(c.Name, c.Version)
}

// plasmaMeta represents the structure of meta/plasma.yaml
type plasmaMeta struct {
	Plasma struct {
		Version string `yaml:"version"`
	} `yaml:"plasma"`
}

// readVersion reads the version from a component's meta/plasma.yaml file.
func readVersion(metaPath string) string {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ""
	}
	var meta plasmaMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return meta.Plasma.Version
}

// Attachment represents a component attached to a chassis path.
type Attachment struct {
	Component string
	Playbook  string
	Chassis   string
}

// LoadFromPlaybooks discovers components from layer playbooks.
// It scans src/<layer>/<layer>.yaml files for role declarations.
func LoadFromPlaybooks(dir string) (Components, error) {
	var components Components

	srcDir := filepath.Join(dir, "src")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		layer := entry.Name()
		playbookPath := filepath.Join(srcDir, layer, layer+".yaml")
		data, err := os.ReadFile(playbookPath)
		if err != nil {
			continue
		}

		// Parse playbook
		var plays []struct {
			Hosts string        `yaml:"hosts"`
			Roles []interface{} `yaml:"roles"`
		}
		if err := yaml.Unmarshal(data, &plays); err != nil {
			continue
		}

		for _, play := range plays {
			for _, r := range play.Roles {
				var roleName string
				switch role := r.(type) {
				case string:
					roleName = role
				case map[string]interface{}:
					if name, ok := role["role"].(string); ok {
						roleName = name
					}
				}
				if roleName != "" {
					// Try to read version from meta/plasma.yaml
					version := ""
					parts := strings.Split(roleName, ".")
					if len(parts) >= 3 {
						// Try src/ first (may have newer changes not yet composed)
						// roles/ structure
						metaPath := filepath.Join(srcDir, parts[0], parts[1], "roles", parts[2], "meta", "plasma.yaml")
						version = readVersion(metaPath)
						// flat structure
						if version == "" {
							metaPath = filepath.Join(srcDir, parts[0], parts[1], parts[2], "meta", "plasma.yaml")
							version = readVersion(metaPath)
						}
						// Fallback to composed directory (for components from packages)
						if version == "" {
							composedDir := filepath.Join(dir, model.MergedSrcDir)
							metaPath = filepath.Join(composedDir, parts[0], parts[1], parts[2], "meta", "plasma.yaml")
							version = readVersion(metaPath)
						}
					}
					components = append(components, Component{
						Name:     roleName,
						Kind:     extractKind(roleName),
						Layer:    layer,
						Version:  version,
						Playbook: playbookPath,
						Chassis:  play.Hosts,
					})
				}
			}
		}
	}

	return components, nil
}

// extractKind extracts the component kind from a full component name.
// e.g., "interaction.applications.dashboards" -> "applications"
func extractKind(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

// LoadFromFilesystem discovers ALL components from the composed output.
// It scans the merged source directory for components with meta/plasma.yaml files.
// These components may or may not be attached to chassis paths.
func LoadFromFilesystem(dir string) (Components, error) {
	srcDir := filepath.Join(dir, model.MergedSrcDir)
	return LoadFromPath(srcDir)
}

// LoadFromPath discovers components from a given base path.
// Auto-detects whether components use roles/ subdirectory structure.
// Valid components must have a meta/plasma.yaml file.
func LoadFromPath(basePath string) (Components, error) {
	var components Components

	layers, err := os.ReadDir(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, layer := range layers {
		if !layer.IsDir() {
			continue
		}
		layerName := layer.Name()

		// Skip hidden directories
		if strings.HasPrefix(layerName, ".") {
			continue
		}

		layerPath := filepath.Join(basePath, layerName)

		// Scan component kinds (applications, services, flows, etc.)
		kinds, err := os.ReadDir(layerPath)
		if err != nil {
			continue
		}

		for _, kind := range kinds {
			if !kind.IsDir() {
				continue
			}
			kindName := kind.Name()

			// Skip non-component directories
			if kindName == "group_vars" || kindName == "host_vars" || strings.HasSuffix(kindName, ".yaml") {
				continue
			}

			kindPath := filepath.Join(layerPath, kindName)

			// Auto-detect roles/ subdirectory structure
			rolesPath := filepath.Join(kindPath, "roles")
			if stat, err := os.Stat(rolesPath); err == nil && stat.IsDir() {
				kindPath = rolesPath
			}

			// Scan component names
			names, err := os.ReadDir(kindPath)
			if err != nil {
				continue
			}

			for _, name := range names {
				if !name.IsDir() {
					continue
				}
				componentName := name.Name()

				// Skip special directories
				if componentName == "roles" || strings.HasPrefix(componentName, ".") {
					continue
				}

				// Verify this is a valid component by checking for meta/plasma.yaml
				metaPath := filepath.Join(kindPath, componentName, "meta", "plasma.yaml")
				if _, err := os.Stat(metaPath); os.IsNotExist(err) {
					continue
				}

				// Component name: layer.kind.name
				fullName := layerName + "." + kindName + "." + componentName
				components = append(components, Component{
					Name:    fullName,
					Kind:    kindName,
					Layer:   layerName,
					Version: readVersion(metaPath),
				})
			}
		}
	}

	return components, nil
}

// LoadAttachments scans playbooks for component attachments to chassis paths.
// If chassisPath is empty, returns all attachments.
// If chassisPath is specified, returns attachments for that path and its children.
func LoadAttachments(dir, chassisPath string) ([]Attachment, error) {
	var attachments []Attachment

	srcDir := filepath.Join(dir, "src")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		playbookPath := filepath.Join(srcDir, entry.Name(), entry.Name()+".yaml")
		data, err := os.ReadFile(playbookPath)
		if err != nil {
			continue
		}

		var plays []struct {
			Hosts string        `yaml:"hosts"`
			Roles []interface{} `yaml:"roles"`
		}
		if err := yaml.Unmarshal(data, &plays); err != nil {
			continue
		}

		for _, play := range plays {
			// Match chassis path filter
			if chassisPath != "" {
				if play.Hosts != chassisPath && !strings.HasPrefix(play.Hosts, chassisPath+".") {
					continue
				}
			}

			for _, r := range play.Roles {
				var roleName string
				switch role := r.(type) {
				case string:
					roleName = role
				case map[string]interface{}:
					if name, ok := role["role"].(string); ok {
						roleName = name
					}
				}
				if roleName != "" {
					attachments = append(attachments, Attachment{
						Component: roleName,
						Playbook:  playbookPath,
						Chassis:   play.Hosts,
					})
				}
			}
		}
	}

	return attachments, nil
}

// Package component provides types and operations for managing platform components.
// Components are logical units (applications, services, flows, etc.) that attach to chassis sections.
package component

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Component represents a platform component discovered from playbooks.
type Component struct {
	Name     string // Full component name (e.g., "interaction.applications.dashboards")
	Kind     string // Component kind (e.g., "applications", "services", "flows")
	Layer    string // Layer (e.g., "interaction", "foundation", "cognition")
	Playbook string // Path to playbook where component is defined
	Section  string // Chassis section where component is attached
}

// Attachment represents a component attached to a chassis section.
type Attachment struct {
	Component string
	Playbook  string
	Section   string
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
					components = append(components, Component{
						Name:     roleName,
						Kind:     extractKind(roleName),
						Layer:    layer,
						Playbook: playbookPath,
						Section:  play.Hosts,
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

// LoadAttachments scans playbooks for component attachments to chassis sections.
// If section is empty, returns all attachments.
// If section is specified, returns attachments for that section and its children.
func LoadAttachments(dir, section string) ([]Attachment, error) {
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
			// Match section filter
			if section != "" {
				if play.Hosts != section && !strings.HasPrefix(play.Hosts, section+".") {
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
						Section:   play.Hosts,
					})
				}
			}
		}
	}

	return attachments, nil
}

package playbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Role represents an Ansible role entry that can be either a simple string
// or an extended map with role name and vars
type Role struct {
	Name string                 // Role name (MRN)
	Vars map[string]interface{} // Optional vars for extended format
}

// UnmarshalYAML handles both string and map role formats
func (r *Role) UnmarshalYAML(node *yaml.Node) error {
	// Try simple string format first
	if node.Kind == yaml.ScalarNode {
		r.Name = node.Value
		return nil
	}

	// Handle map format: {role: name, vars: {...}}
	if node.Kind == yaml.MappingNode {
		var roleMap struct {
			Role string                 `yaml:"role"`
			Vars map[string]interface{} `yaml:"vars"`
		}
		if err := node.Decode(&roleMap); err != nil {
			return err
		}
		r.Name = roleMap.Role
		r.Vars = roleMap.Vars
		return nil
	}

	return fmt.Errorf("invalid role format at line %d", node.Line)
}

// MarshalYAML outputs simple format if no vars, extended format otherwise
func (r Role) MarshalYAML() (interface{}, error) {
	if len(r.Vars) == 0 {
		return r.Name, nil
	}
	return map[string]interface{}{
		"role": r.Name,
		"vars": r.Vars,
	}, nil
}

// Play represents a play in a layer playbook
type Play struct {
	Hosts          string   `yaml:"hosts"`
	Serial         int      `yaml:"serial,omitempty"`
	AnyErrorsFatal bool     `yaml:"any_errors_fatal,omitempty"`
	Roles          []Role   `yaml:"roles"`
	Tags           []string `yaml:"tags,omitempty"`
}

// ExtractLayer gets the layer name from an MRN
func ExtractLayer(mrn string) string {
	parts := strings.Split(mrn, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// FindPlaybook locates the layer playbook file
func FindPlaybook(source, layer string) (string, error) {
	candidates := []string{
		filepath.Join(source, "src", layer, layer+".yaml"),
		filepath.Join(source, layer, layer+".yaml"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("layer playbook not found for %q (tried: %v)", layer, candidates)
}

// Load reads and parses the playbook YAML
func Load(path string) ([]Play, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read playbook: %w", err)
	}

	var plays []Play
	if err := yaml.Unmarshal(data, &plays); err != nil {
		return nil, fmt.Errorf("failed to parse playbook: %w", err)
	}

	return plays, nil
}

// Save writes the playbook back to disk
func Save(path string, plays []Play) error {
	data, err := yaml.Marshal(plays)
	if err != nil {
		return fmt.Errorf("failed to marshal playbook: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write playbook: %w", err)
	}

	return nil
}

// AddRole adds the component to the appropriate chassis play
func AddRole(plays []Play, component, chassis string) ([]Play, bool) {
	for i, play := range plays {
		if play.Hosts == chassis {
			for _, role := range play.Roles {
				if role.Name == component {
					return plays, false // already attached
				}
			}
			plays[i].Roles = append(plays[i].Roles, Role{Name: component})
			return plays, true
		}
	}

	// Create new play for this chassis
	newPlay := Play{
		Hosts:          chassis,
		AnyErrorsFatal: true,
		Roles:          []Role{{Name: component}},
	}
	return append(plays, newPlay), true
}

// RemoveRole removes the component from the chassis play
func RemoveRole(plays []Play, component, chassis string) ([]Play, bool) {
	for i, play := range plays {
		if play.Hosts == chassis {
			newRoles := make([]Role, 0, len(play.Roles))
			found := false
			for _, role := range play.Roles {
				if role.Name == component {
					found = true
					continue
				}
				newRoles = append(newRoles, role)
			}

			if !found {
				return plays, false
			}

			plays[i].Roles = newRoles

			// Remove empty play
			if len(newRoles) == 0 {
				plays = append(plays[:i], plays[i+1:]...)
			}
			return plays, true
		}
	}

	return plays, false
}

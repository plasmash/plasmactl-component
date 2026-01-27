package playbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Play represents a play in a layer playbook
type Play struct {
	Hosts          string   `yaml:"hosts"`
	AnyErrorsFatal bool     `yaml:"any_errors_fatal,omitempty"`
	Roles          []string `yaml:"roles"`
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
				if role == component {
					return plays, false // already attached
				}
			}
			plays[i].Roles = append(plays[i].Roles, component)
			return plays, true
		}
	}

	// Create new play for this chassis
	newPlay := Play{
		Hosts:          chassis,
		AnyErrorsFatal: true,
		Roles:          []string{component},
	}
	return append(plays, newPlay), true
}

// RemoveRole removes the component from the chassis play
func RemoveRole(plays []Play, component, chassis string) ([]Play, bool) {
	for i, play := range plays {
		if play.Hosts == chassis {
			newRoles := make([]string, 0, len(play.Roles))
			found := false
			for _, role := range play.Roles {
				if role == component {
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

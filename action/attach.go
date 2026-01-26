package action

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"gopkg.in/yaml.v3"
)

// Attach implements component:attach command
type Attach struct {
	action.WithLogger
	action.WithTerm

	Component string
	Chassis   string
	Source    string
}

// Execute runs the attach action
func (a *Attach) Execute() error {
	layer := extractLayer(a.Component)
	if layer == "" {
		return fmt.Errorf("invalid component MRN %q: cannot extract layer", a.Component)
	}

	playbookPath, err := findPlaybook(a.Source, layer)
	if err != nil {
		return err
	}

	plays, err := loadPlaybook(playbookPath)
	if err != nil {
		return err
	}

	plays, attached := addToPlay(plays, a.Component, a.Chassis)
	if !attached {
		a.Term().Warning().Printfln("Component %s already attached to %s", a.Component, a.Chassis)
		return nil
	}

	if err := savePlaybook(playbookPath, plays); err != nil {
		return err
	}

	a.Term().Success().Printfln("Attached %s to %s", a.Component, a.Chassis)
	return nil
}

// Play represents a play in the layer playbook
type Play struct {
	Hosts          string   `yaml:"hosts"`
	AnyErrorsFatal bool     `yaml:"any_errors_fatal,omitempty"`
	Roles          []string `yaml:"roles"`
}

// extractLayer gets the layer name from an MRN
func extractLayer(mrn string) string {
	parts := strings.Split(mrn, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

// findPlaybook locates the layer playbook file
func findPlaybook(source, layer string) (string, error) {
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

// loadPlaybook reads and parses the playbook YAML
func loadPlaybook(path string) ([]Play, error) {
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

// savePlaybook writes the playbook back to disk
func savePlaybook(path string, plays []Play) error {
	data, err := yaml.Marshal(plays)
	if err != nil {
		return fmt.Errorf("failed to marshal playbook: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write playbook: %w", err)
	}

	return nil
}

// addToPlay adds the component to the appropriate chassis play
func addToPlay(plays []Play, component, chassis string) ([]Play, bool) {
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

// removeFromPlay removes the component from the chassis play
func removeFromPlay(plays []Play, component, chassis string) ([]Play, bool) {
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

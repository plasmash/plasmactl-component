package component

import (
	"sort"

	"github.com/plasmash/plasmactl-chassis/pkg/chassis"
)

// Components is a collection of Component.
type Components []Component

// Find returns the component with the given name, or nil if not found.
func (cs Components) Find(name string) *Component {
	for i := range cs {
		if cs[i].Name == name {
			return &cs[i]
		}
	}
	return nil
}

// Names returns a list of all component names.
func (cs Components) Names() []string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Name
	}
	return names
}

// ByKind returns components filtered by kind.
func (cs Components) ByKind(kind string) Components {
	var result Components
	for _, c := range cs {
		if c.Kind == kind {
			result = append(result, c)
		}
	}
	return result
}

// ByLayer returns components filtered by layer.
func (cs Components) ByLayer(layer string) Components {
	var result Components
	for _, c := range cs {
		if c.Layer == layer {
			result = append(result, c)
		}
	}
	return result
}

// Attachments returns a map of component name to chassis paths.
// Unlike node allocations, component attachments don't use distribution -
// they are explicit bindings defined in playbooks.
//
// The chassis parameter is provided for API consistency and future use
// (e.g., validating chassis paths exist, computing inherited attachments).
//
// Returns: component_name → []chassisPaths
func (cs Components) Attachments(_ *chassis.Chassis) map[string][]string {
	result := make(map[string][]string)

	for _, c := range cs {
		if c.Chassis != "" {
			result[c.Name] = appendUnique(result[c.Name], c.Chassis)
		}
	}

	// Sort chassis paths for consistent output
	for name := range result {
		sort.Strings(result[name])
	}

	return result
}

// ForChassis returns components attached to a chassis path or its children.
func (cs Components) ForChassis(chassisPath string) Components {
	var result Components
	for _, c := range cs {
		if c.Chassis == chassisPath || chassis.IsDescendantOf(c.Chassis, chassisPath) {
			result = append(result, c)
		}
	}
	return result
}

// appendUnique appends value to slice if not already present.
func appendUnique(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice
		}
	}
	return append(slice, value)
}

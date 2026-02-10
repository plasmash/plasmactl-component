package sync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	tplVersionGet = "failed to get component version (%s)"
	tplVersionSet = "failed to update component version (%s)"
)

// PrepareComponentName creates a dot-notation component name from parts.
// Example: PrepareComponentName("foundation", "applications", "auth") -> "foundation.applications.auth"
func PrepareComponentName(layer, kind, name string) string {
	return fmt.Sprintf("%s.%s.%s", layer, kind, name)
}

// ConvertNameToPath transforms component name (dot notation) to filesystem path.
// Example: "foundation.applications.auth" -> "foundation/applications/auth"
func ConvertNameToPath(name string) (string, error) {
	parts := strings.Split(name, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid component name format (expected: layer.kind.name)")
	}
	return filepath.Join(parts[0], parts[1], parts[2]), nil
}

// Component represents a platform component
type Component struct {
	name       string
	pathPrefix string
	platform   string
	kind       string
	role       string
}

// NewComponent returns new [Component] instance.
// Accepts dot notation: "foundation.applications.auth"
func NewComponent(name, prefix string) (*Component, error) {
	parts := strings.Split(name, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid component name %q (expected: layer.kind.name)", name)
	}

	return &Component{
		name:       name,
		pathPrefix: prefix,
		platform:   parts[0],
		kind:       parts[1],
		role:       parts[2],
	}, nil
}

// GetName returns a machine component name.
func (c *Component) GetName() string {
	return c.name
}

// GetPlatform returns a component platform.
func (c *Component) GetPlatform() string {
	return c.platform
}

// GetKind returns a component kind.
func (c *Component) GetKind() string {
	return c.kind
}

// GetRole returns a component name.
func (c *Component) GetRole() string {
	return c.role
}

// IsValidComponent checks if component has meta file.
func (c *Component) IsValidComponent() bool {
	metaPath := c.getRealMetaPath()
	_, err := os.Stat(metaPath)

	return !os.IsNotExist(err)
}

func (c *Component) getRealMetaPath() string {
	meta := c.BuildMetaPath()
	return filepath.Join(c.pathPrefix, meta)
}

// BuildMetaPath returns common path to component meta.
func (c *Component) BuildMetaPath() string {
	parts := strings.Split(c.GetName(), ".")
	meta := filepath.Join(parts[0], parts[1], parts[2], "meta", "plasma.yaml")
	return meta
}

// GetVersion retrieves the version of the component from the plasma.yaml
func (c *Component) GetVersion() (string, []string, error) {
	var debug []string
	metaFile := c.getRealMetaPath()
	if _, err := os.Stat(metaFile); err == nil {
		data, errRead := os.ReadFile(filepath.Clean(metaFile))
		if errRead != nil {
			debug = append(debug, errRead.Error())
			return "", debug, fmt.Errorf(tplVersionGet, metaFile)
		}

		var meta map[string]any
		errUnmarshal := yaml.Unmarshal(data, &meta)
		if errUnmarshal != nil {
			debug = append(debug, errUnmarshal.Error())
			return "", debug, fmt.Errorf(tplVersionGet, metaFile)
		}

		version := GetMetaVersion(meta)
		if version == "" {
			debug = append(debug, fmt.Sprintf("Empty meta file %s version, return empty string as version", metaFile))
		}

		return version, debug, nil
	}

	return "", debug, fmt.Errorf(tplVersionGet, metaFile)
}

// GetMetaVersion searches for version in meta data.
func GetMetaVersion(meta map[string]any) string {
	if plasma, ok := meta["plasma"].(map[string]any); ok {
		version := plasma["version"]
		if version == nil {
			version = ""
		}
		val, okConversion := version.(string)
		if okConversion {
			return val
		}

		return fmt.Sprint(version)
	}

	return ""
}

// GetBaseVersion returns component version without `-` if any.
func (c *Component) GetBaseVersion() (string, string, []string, error) {
	var debug []string
	version, debugMessages, err := c.GetVersion()
	debug = append(debug, debugMessages...)
	if err != nil {
		return "", "", debug, err
	}

	split := strings.Split(version, "-")
	if len(split) > 2 {
		debug = append(debug, fmt.Sprintf("Component %s has incorrect format %s", c.GetName(), version))
	}

	return split[0], version, debug, nil
}

// UpdateVersion updates the version of the component in the plasma.yaml file
func (c *Component) UpdateVersion(version string) ([]string, error) {
	var debug []string
	metaFilepath := c.getRealMetaPath()
	if _, err := os.Stat(metaFilepath); err == nil {
		data, errRead := os.ReadFile(filepath.Clean(metaFilepath))
		if errRead != nil {
			debug = append(debug, errRead.Error())
			return debug, fmt.Errorf(tplVersionSet, metaFilepath)
		}

		var b bytes.Buffer
		var meta map[string]any
		errUnmarshal := yaml.Unmarshal(data, &meta)
		if errUnmarshal != nil {
			debug = append(debug, errUnmarshal.Error())
			return debug, fmt.Errorf(tplVersionSet, metaFilepath)
		}

		if plasma, ok := meta["plasma"].(map[string]any); ok {
			plasma["version"] = version
		} else {
			meta["plasma"] = map[string]any{"version": version}
		}

		yamlEncoder := yaml.NewEncoder(&b)
		yamlEncoder.SetIndent(2)
		errEncode := yamlEncoder.Encode(&meta)
		if errEncode != nil {
			debug = append(debug, errEncode.Error())
			return debug, fmt.Errorf(tplVersionSet, metaFilepath)
		}

		errWrite := os.WriteFile(metaFilepath, b.Bytes(), 0600)
		if errWrite != nil {
			debug = append(debug, errWrite.Error())
			return debug, fmt.Errorf(tplVersionSet, metaFilepath)
		}

		return debug, nil
	}

	return debug, fmt.Errorf(tplVersionSet, metaFilepath)
}

// BuildComponentFromPath builds a new instance of Component from the given path.
func BuildComponentFromPath(path, pathPrefix string) *Component {
	platform, kind, role, err := ProcessComponentPath(path)
	if err != nil || (platform == "" || kind == "" || role == "") {
		return nil
	}

	component, err := NewComponent(PrepareComponentName(platform, kind, role), pathPrefix)
	if err != nil {
		return nil
	}
	if !component.IsValidComponent() {
		return nil
	}
	return component
}

// ProcessComponentPath splits component path onto platform, kind and role.
func ProcessComponentPath(path string) (string, string, string, error) {
	parts := strings.Split(path, "/")
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2], nil
	}

	return "", "", "", errors.New("empty component path")
}

// IsUpdatableKind checks if component kind is in [Kinds] range.
func IsUpdatableKind(kind string) bool {
	_, ok := Kinds[kind]
	return ok
}

// OrderedMap represents generic struct with map and order keys.
type OrderedMap[T any] struct {
	keys []string
	dict map[string]T
}

// NewOrderedMap returns a new instance of [OrderedMap].
func NewOrderedMap[T any]() *OrderedMap[T] {
	return &OrderedMap[T]{
		keys: make([]string, 0),
		dict: make(map[string]T),
	}
}

// Set a value in the [OrderedMap].
func (m *OrderedMap[T]) Set(key string, value T) {
	if _, ok := m.dict[key]; !ok {
		m.keys = append(m.keys, key)
	}

	m.dict[key] = value
}

// Unset a value from the [OrderedMap].
func (m *OrderedMap[T]) Unset(key string) {
	if _, ok := m.dict[key]; ok {
		index := -1
		for i, item := range m.keys {
			if item == key {
				index = i
			}
		}
		if index != -1 {
			m.keys = append(m.keys[:index], m.keys[index+1:]...)
		}

	}

	delete(m.dict, key)
}

// Get a value from the [OrderedMap].
func (m *OrderedMap[T]) Get(key string) (T, bool) {
	val, ok := m.dict[key]
	return val, ok
}

// Keys returns the ordered keys from the [OrderedMap].
func (m *OrderedMap[T]) Keys() []string {
	var keys []string
	keys = append(keys, m.keys...)

	return keys
}

// OrderBy updates the order of keys in the [OrderedMap] based on the orderList.
func (m *OrderedMap[T]) OrderBy(orderList []string) {
	var newKeys []string
	var remainingKeys []string

keysLoop:
	for _, key := range m.keys {
		isInOrderList := false
		for _, orderKey := range orderList {
			if key == orderKey {
				isInOrderList = true
				continue keysLoop
			}
		}

		if !isInOrderList {
			remainingKeys = append(remainingKeys, key)
		}
	}

	for _, item := range orderList {
		_, ok := m.Get(item)
		if ok {
			newKeys = append(newKeys, item)
		}
	}

	newKeys = append(newKeys, remainingKeys...)
	m.keys = newKeys
}

// SortKeysAlphabetically sorts internal keys alphabetically.
func (m *OrderedMap[T]) SortKeysAlphabetically() {
	sort.Strings(m.keys)
}

// Len returns the length of the [OrderedMap].
func (m *OrderedMap[T]) Len() int {
	return len(m.keys)
}

// ToList converts map to ordered list [OrderedMap].
func (m *OrderedMap[T]) ToList() []T {
	var list []T
	for _, key := range m.keys {
		list = append(list, m.dict[key])
	}
	return list
}

// ToDict returns copy of [OrderedMap] dictionary.
func (m *OrderedMap[T]) ToDict() map[string]T {
	dict := make(map[string]T)
	for key, value := range m.dict {
		dict[key] = value
	}
	return dict
}

// GetUsedComponents returns list of used components.
func (i *Inventory) GetUsedComponents() map[string]bool {
	if !i.componentsUsageCalculated {
		panic("use inventory.CalculateComponentsUsage first")
	}

	return i.usedComponents
}

// CalculateComponentsUsage parse platform playbooks and determine components used in platform.
func (i *Inventory) CalculateComponentsUsage() error {
	file, err := os.ReadFile(filepath.Join(i.sourceDir, "platform/platform.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("platform/platform.yaml playbook doesn't exist")
		}

		return err
	}

	var platformData []any
	err = yaml.Unmarshal(file, &platformData)
	if err != nil {
		return err
	}

	var playbooks []string
	components := make(map[string]bool)

	for _, item := range platformData {
		if m, ok := item.(map[string]any); ok {
			for k, val := range m {
				if k == "import_playbook" {
					playbookName, okV := val.(string)
					if okV {
						cleanPath := filepath.Clean(strings.ReplaceAll(playbookName, "../", ""))
						playbooks = append(playbooks, filepath.Join(i.sourceDir, cleanPath))
					}
				}

				extractPlaybookRoles(components, k, val)
			}
		}
	}

	for _, playbook := range playbooks {
		var playbookData []any
		file, err = os.ReadFile(filepath.Clean(playbook))
		if err != nil {
			return err
		}

		err = yaml.Unmarshal(file, &playbookData)
		if err != nil {
			return err
		}

		for _, item := range playbookData {
			if m, ok := item.(map[string]any); ok {
				for k, val := range m {
					extractPlaybookRoles(components, k, val)
				}
			}
		}
	}

	usedComponentsWithDependencies := make(map[string]bool)
	for c := range components {
		deps := i.GetRequiresComponents(c, -1)

		usedComponentsWithDependencies[c] = true
		for d := range deps {
			usedComponentsWithDependencies[d] = true
		}
	}

	i.usedComponents = usedComponentsWithDependencies
	i.componentsUsageCalculated = true

	return nil
}

func extractPlaybookRoles(result map[string]bool, k string, val any) {
	if k != "roles" {
		return
	}

	if s, ok := val.([]any); ok {
		for _, i := range s {
			if v, okV := i.(string); okV {
				result[v] = true
				continue
			}

			if m, okM := i.(map[string]any); okM {
				role, okR := m["role"]
				if !okR {
					return
				}

				if r, okV := role.(string); okV {
					result[r] = true

					continue
				}
			}
		}
	}
}

// Package sync contains tools to provide bump propagation.
package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/stevenle/topsort"
	"gopkg.in/yaml.v3"
)

const (
	invalidPasswordErrText = "invalid password"

	rootPlatform = "platform"
	vaultFile    = "vault.yaml"
)

// InventoryExcluded is list of excluded files and folders from inventory.
var InventoryExcluded = []string{
	".git",
	".plasma",
	".plasmactl",
	".gitlab-ci.yml",
	"ansible_collections",
	"scripts/ci/.gitlab-ci.platform.yaml",
	"venv",
	"__pycache__",
}

// Kinds are list of component kinds which version can be propagated.
var Kinds = map[string]struct{}{
	"applications": {},
	"services":     {},
	"softwares":    {},
	"executors":    {},
	"flows":        {},
	"skills":       {},
	"functions":    {},
	"libraries":    {},
	"entities":     {},
}

// Inventory represents the inventory used in the application to search and collect components and variable components.
type Inventory struct {
	// services
	fc  *FilesCrawler
	log *launchr.Logger

	//internal
	componentsMap   *OrderedMap[*Component]
	requiredBy      map[string]*OrderedMap[bool] // semantic dependencies (from dependencies.yaml)
	requires        map[string]*OrderedMap[bool] // semantic dependencies (from dependencies.yaml)
	buildRequiredBy map[string]*OrderedMap[bool] // build dependencies (from main.yaml)
	buildRequires   map[string]*OrderedMap[bool] // build dependencies (from main.yaml)
	topOrder        []string

	componentsUsageCalculated bool
	usedComponents            map[string]bool

	variablesUsageCalculated        bool
	variableVariablesDependencyMap  map[string]map[string]*VariableDependency
	variableComponentsDependencyMap map[string]map[string][]string

	// options
	sourceDir string
}

// NewInventory creates a new instance of Inventory with the provided vault password.
// It then calls the Init method of the Inventory to build the components graph and returns
// the initialized Inventory or any error that occurred during initialization.
func NewInventory(sourceDir string, log *launchr.Logger) (*Inventory, error) {
	inv := &Inventory{
		sourceDir:                       sourceDir,
		fc:                              NewFilesCrawler(sourceDir),
		log:                             log,
		componentsMap:                   NewOrderedMap[*Component](),
		requiredBy:                      make(map[string]*OrderedMap[bool]),
		requires:                        make(map[string]*OrderedMap[bool]),
		buildRequiredBy:                 make(map[string]*OrderedMap[bool]),
		buildRequires:                   make(map[string]*OrderedMap[bool]),
		variableVariablesDependencyMap:  make(map[string]map[string]*VariableDependency),
		variableComponentsDependencyMap: make(map[string]map[string][]string),
	}

	err := inv.Init()

	if err != nil {
		err = fmt.Errorf("inventory init error (%s) > %w", sourceDir, err)
	}

	return inv, err
}

// Init initializes the Inventory by building the components graph.
// It returns an error if there was an issue while building the graph.
func (i *Inventory) Init() error {
	err := i.buildComponentsGraph()
	return err
}

func (i *Inventory) buildComponentsGraph() error {
	err := filepath.Walk(i.sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return nil
		}

		relPath := strings.TrimPrefix(path, i.sourceDir+"/")
		for _, d := range InventoryExcluded {
			if strings.Contains(relPath, d) {
				return nil
			}
		}

		entity := strings.ToLower(filepath.Base(relPath))
		ext := filepath.Ext(entity)
		dir := filepath.Dir(relPath)

		isMetaDir := strings.HasSuffix(dir, "/meta")
		isTasksDir := strings.HasSuffix(dir, "/tasks")

		if (!isMetaDir && !isTasksDir) || (ext != ".yaml" && ext != ".yml") {
			return nil
		}

		if isMetaDir && entity == "plasma.yaml" {
			component := BuildComponentFromPath(relPath, i.sourceDir)
			if component == nil || !component.IsValidComponent() {
				return nil
			}

			componentName := component.GetName()
			i.componentsMap.Set(componentName, component)
		} else if isTasksDir {
			component := BuildComponentFromPath(relPath, i.sourceDir)
			if component == nil || !component.IsValidComponent() {
				return nil
			}

			componentName := component.GetName()
			if _, exists := i.componentsMap.Get(componentName); !exists {
				i.componentsMap.Set(componentName, component)
			}

			data, errRead := os.ReadFile(filepath.Clean(path))
			if errRead != nil {
				return errRead
			}

			var tasks []map[string]any
			err = yaml.Unmarshal(data, &tasks)
			if err != nil {
				return fmt.Errorf("%s > %w", path, err)
			}

			if len(tasks) == 0 {
				return nil
			}

			// Choose target maps based on file type
			// dependencies.yaml → semantic dependencies
			// all other tasks/*.yaml → build dependencies
			isSemanticDeps := entity == "dependencies.yaml"
			var requiresMap map[string]*OrderedMap[bool]
			var requiredByMap map[string]*OrderedMap[bool]
			if isSemanticDeps {
				requiresMap = i.requires
				requiredByMap = i.requiredBy
			} else {
				requiresMap = i.buildRequires
				requiredByMap = i.buildRequiredBy
			}

			if requiresMap[componentName] == nil {
				requiresMap[componentName] = NewOrderedMap[bool]()
			}

			for _, entry := range tasks {
				if r, ok := entry["include_role"].(map[string]any); ok {
					if n, ok := r["name"].(string); ok && n != "" {
						depName := n // use dot notation as-is
						if requiredByMap[depName] == nil {
							requiredByMap[depName] = NewOrderedMap[bool]()
						}

						requiredByMap[depName].Set(componentName, true)
						requiresMap[componentName].Set(depName, true)
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	platformItems := NewOrderedMap[bool]()
	for componentName := range i.requiredBy {
		if _, ok := i.requires[componentName]; !ok {
			platformItems.Set(componentName, true)
		}
	}

	i.requiredBy[rootPlatform] = platformItems
	graph := topsort.NewGraph()
	for platform, components := range i.requiredBy {
		for _, component := range components.Keys() {
			graph.AddNode(component)
			edgeErr := graph.AddEdge(platform, component)
			if edgeErr != nil {
				return edgeErr
			}
		}
	}

	order, err := graph.TopSort(rootPlatform)
	if err != nil {
		return err
	}

	// reverse order to have platform at top
	for y, j := 0, len(order)-1; y < j; y, j = y+1, j-1 {
		order[y], order[j] = order[j], order[y]
	}

	i.topOrder = order
	i.componentsMap.OrderBy(order)

	return nil
}

// GetComponentsMap returns map of all components found in source dir.
func (i *Inventory) GetComponentsMap() *OrderedMap[*Component] {
	return i.componentsMap
}

// GetComponentsOrder returns the order of components in the inventory.
func (i *Inventory) GetComponentsOrder() []string {
	return i.topOrder
}

// GetRequiredByMap returns the required by map, which represents the `required by` dependencies between components in the Inventory.
func (i *Inventory) GetRequiredByMap() map[string]*OrderedMap[bool] {
	return i.requiredBy
}

// GetRequiresMap returns the map, which represents the 'requires' dependencies between components in the Inventory.
func (i *Inventory) GetRequiresMap() map[string]*OrderedMap[bool] {
	return i.requires
}

// GetRequiredByComponents returns list of components which depend on argument component (directly or not).
func (i *Inventory) GetRequiredByComponents(componentName string, depth int8) map[string]bool {
	return i.lookupDependencies(componentName, i.GetRequiredByMap(), depth)
}

// GetRequiresComponents returns list of components which are used by argument component (directly or not).
func (i *Inventory) GetRequiresComponents(componentName string, depth int8) map[string]bool {
	return i.lookupDependencies(componentName, i.GetRequiresMap(), depth)
}

// GetBuildRequiredByMap returns the build required by map (from main.yaml).
func (i *Inventory) GetBuildRequiredByMap() map[string]*OrderedMap[bool] {
	return i.buildRequiredBy
}

// GetBuildRequiresMap returns the build requires map (from main.yaml).
func (i *Inventory) GetBuildRequiresMap() map[string]*OrderedMap[bool] {
	return i.buildRequires
}

// GetBuildRequiredByComponents returns list of components which use this component for build (directly or not).
func (i *Inventory) GetBuildRequiredByComponents(componentName string, depth int8) map[string]bool {
	return i.lookupDependencies(componentName, i.GetBuildRequiredByMap(), depth)
}

// GetBuildRequiresComponents returns list of build components required by argument component (directly or not).
func (i *Inventory) GetBuildRequiresComponents(componentName string, depth int8) map[string]bool {
	return i.lookupDependencies(componentName, i.GetBuildRequiresMap(), depth)
}

func (i *Inventory) lookupDependencies(componentName string, componentsMap map[string]*OrderedMap[bool], depth int8) map[string]bool {
	result := make(map[string]bool)
	if m, ok := componentsMap[componentName]; ok {
		for _, item := range m.Keys() {
			result[item] = true
			i.lookupDependenciesRecursively(item, componentsMap, result, 1, depth)
		}
	}

	return result
}

func (i *Inventory) lookupDependenciesRecursively(componentName string, componentsMap map[string]*OrderedMap[bool], result map[string]bool, depth, limit int8) {
	if depth == limit {
		return
	}

	if m, ok := componentsMap[componentName]; ok {
		for _, item := range m.Keys() {
			result[item] = true
			i.lookupDependenciesRecursively(item, componentsMap, result, depth+1, limit)
		}
	}
}

// GetChangedComponents returns an OrderedMap containing the components that have been modified, based on the provided list of modified files.
// It iterates over the modified files, builds a component from each file path, and adds it to the result map if it is not already present.
func (i *Inventory) GetChangedComponents(files []string) *OrderedMap[*Component] {
	components := NewOrderedMap[*Component]()
	for _, path := range files {
		component := BuildComponentFromPath(path, i.sourceDir)
		if component == nil {
			continue
		}
		if _, ok := components.Get(component.GetName()); ok {
			continue
		}
		components.Set(component.GetName(), component)
	}

	return components
}

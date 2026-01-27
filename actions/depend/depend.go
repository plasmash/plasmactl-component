package depend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/sync"
	"gopkg.in/yaml.v3"
)

// Depend implements component:depend command
type Depend struct {
	action.WithLogger
	action.WithTerm

	// Arguments
	Target     string
	Operations []string

	// Show options
	Source string
	Path   bool // show paths instead of MRNs
	Tree   bool // show tree-like output
	Depth  int8 // recursion depth limit
}

// DependOp represents a parsed dependency operation
type DependOp struct {
	Type   string // "add", "remove", "replace"
	Dep    string
	NewVal string // only for replace
}

// Execute runs the depend action
func (d *Depend) Execute() error {
	// No operations = show mode
	if len(d.Operations) == 0 {
		return d.executeShow()
	}

	// Parse and execute operations
	return d.executeOperations()
}

// executeShow displays dependencies (original behavior)
func (d *Depend) executeShow() error {
	searchMrn := d.Target
	_, errConvert := sync.ConvertMRNtoPath(searchMrn)

	if errConvert != nil {
		r := sync.BuildResourceFromPath(d.Target, d.Source)
		if r == nil {
			return fmt.Errorf("not valid resource %q", d.Target)
		}

		searchMrn = r.GetName()
	}

	var header string
	if d.Path {
		header, _ = sync.ConvertMRNtoPath(searchMrn)
	} else {
		header = searchMrn
	}

	inv, err := sync.NewInventory(d.Source, d.Log())
	if err != nil {
		return err
	}

	parents := inv.GetRequiredByResources(searchMrn, d.Depth)
	if len(parents) > 0 {
		d.Term().Info().Println("Dependent resources:")
		if d.Tree {
			var parentsTree forwardTree = inv.GetRequiredByMap()
			parentsTree.print(d.Term(), header, "", 1, d.Depth, searchMrn, d.Path)
		} else {
			d.printList(parents, d.Path)
		}
	}

	children := inv.GetDependsOnResources(searchMrn, d.Depth)
	if len(children) > 0 {
		d.Term().Info().Println("Dependencies:")
		if d.Tree {
			var childrenTree forwardTree = inv.GetDependsOnMap()
			childrenTree.print(d.Term(), header, "", 1, d.Depth, searchMrn, d.Path)
		} else {
			d.printList(children, d.Path)
		}
	}

	if len(parents) == 0 && len(children) == 0 {
		d.Term().Info().Println("No dependencies found")
		d.Term().Println()
		d.Term().Info().Println("Tip: DEP (add), DEP- (remove), OLD/NEW (replace)")
	}

	return nil
}

// executeOperations applies kubectl-style operations
func (d *Depend) executeOperations() error {
	targetPath, err := d.resolveTargetPath()
	if err != nil {
		return err
	}

	depsFile := filepath.Join(targetPath, "tasks", "dependencies.yaml")
	deps, err := d.loadDependencies(depsFile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load dependencies: %w", err)
	}

	// Parse operations
	ops := d.parseOperations()
	modified := false

	for _, op := range ops {
		depMrn, err := d.resolveDependencyMRN(op.Dep)
		if err != nil {
			return err
		}

		switch op.Type {
		case "add":
			if d.addDep(&deps, depMrn) {
				d.Term().Success().Printfln("Added: %s", depMrn)
				modified = true
			} else {
				d.Term().Warning().Printfln("Already exists: %s", depMrn)
			}
		case "remove":
			if d.removeDep(&deps, depMrn) {
				d.Term().Success().Printfln("Removed: %s", depMrn)
				modified = true
			} else {
				d.Term().Warning().Printfln("Not found: %s", depMrn)
			}
		case "replace":
			newMrn, err := d.resolveDependencyMRN(op.NewVal)
			if err != nil {
				return err
			}
			removed := d.removeDep(&deps, depMrn)
			added := d.addDep(&deps, newMrn)
			if removed || added {
				d.Term().Success().Printfln("Replaced: %s → %s", depMrn, newMrn)
				modified = true
			} else {
				d.Term().Warning().Printfln("No change: %s → %s", depMrn, newMrn)
			}
		}
	}

	if !modified {
		return nil
	}

	sort.Strings(deps)

	if err := d.saveDependencies(depsFile, deps); err != nil {
		return fmt.Errorf("failed to save dependencies: %w", err)
	}

	return nil
}

// parseOperations parses kubectl-style operations
func (d *Depend) parseOperations() []DependOp {
	var ops []DependOp

	for _, arg := range d.Operations {
		switch {
		case strings.Contains(arg, "/"):
			// Replace: old/new
			parts := strings.SplitN(arg, "/", 2)
			ops = append(ops, DependOp{
				Type:   "replace",
				Dep:    parts[0],
				NewVal: parts[1],
			})
		case strings.HasSuffix(arg, "-"):
			// Remove: dep-
			ops = append(ops, DependOp{
				Type: "remove",
				Dep:  strings.TrimSuffix(arg, "-"),
			})
		default:
			// Add: dep
			ops = append(ops, DependOp{
				Type: "add",
				Dep:  arg,
			})
		}
	}

	return ops
}

// addDep adds a dependency, returns true if added
func (d *Depend) addDep(deps *[]string, dep string) bool {
	for _, existing := range *deps {
		if existing == dep {
			return false
		}
	}
	*deps = append(*deps, dep)
	return true
}

// removeDep removes a dependency, returns true if removed
func (d *Depend) removeDep(deps *[]string, dep string) bool {
	for i, existing := range *deps {
		if existing == dep {
			*deps = append((*deps)[:i], (*deps)[i+1:]...)
			return true
		}
	}
	return false
}

// resolveTargetPath converts target to a filesystem path
func (d *Depend) resolveTargetPath() (string, error) {
	// Try as path first
	if _, err := os.Stat(d.Target); err == nil {
		return d.Target, nil
	}

	// Try converting from MRN
	path, err := sync.ConvertMRNtoPath(d.Target)
	if err != nil {
		return "", fmt.Errorf("cannot resolve target %q: %w", d.Target, err)
	}

	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("target path does not exist: %s", path)
	}

	return path, nil
}

// resolveDependencyMRN converts dependency to MRN format
func (d *Depend) resolveDependencyMRN(dep string) (string, error) {
	// Check if it's already an MRN
	if _, err := sync.ConvertMRNtoPath(dep); err == nil {
		return dep, nil
	}

	// Try to build resource from path
	r := sync.BuildResourceFromPath(dep, ".")
	if r != nil {
		return r.GetName(), nil
	}

	return "", fmt.Errorf("cannot resolve dependency %q to MRN", dep)
}

// loadDependencies reads the dependencies.yaml file
func (d *Depend) loadDependencies(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var content struct {
		Dependencies []string `yaml:"dependencies"`
	}

	if err := yaml.Unmarshal(data, &content); err != nil {
		// Try as simple list
		var deps []string
		if err2 := yaml.Unmarshal(data, &deps); err2 != nil {
			return nil, fmt.Errorf("failed to parse dependencies: %w", err)
		}
		return deps, nil
	}

	return content.Dependencies, nil
}

// saveDependencies writes the dependencies.yaml file
func (d *Depend) saveDependencies(path string, deps []string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	content := struct {
		Dependencies []string `yaml:"dependencies"`
	}{
		Dependencies: deps,
	}

	data, err := yaml.Marshal(&content)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (d *Depend) printList(items map[string]bool, toPath bool) {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, item := range keys {
		res := item
		if toPath {
			res, _ = sync.ConvertMRNtoPath(res)
		}

		d.Term().Print(res + "\n")
	}
}

type forwardTree map[string]*sync.OrderedMap[bool]

func (t forwardTree) print(printer *launchr.Terminal, header, indent string, depth, limit int8, parent string, toPath bool) {
	if indent == "" {
		printer.Printfln(header)
	}

	if depth == limit {
		return
	}

	children, ok := t[parent]
	if !ok {
		return
	}

	keys := children.Keys()
	sort.Strings(keys)

	for i, node := range keys {
		isLast := i == len(keys)-1
		var newIndent, edge string

		if isLast {
			newIndent = indent + "    "
			edge = "└── "
		} else {
			newIndent = indent + "│   "
			edge = "├── "
		}

		value := node
		if toPath {
			value, _ = sync.ConvertMRNtoPath(value)
		}

		printer.Printfln(indent + edge + value)
		t.print(printer, "", newIndent, depth+1, limit, node, toPath)
	}
}

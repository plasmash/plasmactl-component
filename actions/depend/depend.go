package depend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/sync"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
	"gopkg.in/yaml.v3"
)

// DependOpResult represents the result of a single dependency operation.
type DependOpResult struct {
	Type   string `json:"type"`
	Dep    string `json:"dep"`
	NewDep string `json:"new_dep,omitempty"`
	Applied bool  `json:"applied"`
}

// DependResult is the structured result of component:depend.
type DependResult struct {
	Target     string           `json:"target"`
	Mode       string           `json:"mode"`
	Requires   []string         `json:"requires,omitempty"`
	RequiredBy []string         `json:"required_by,omitempty"`
	Operations []DependOpResult `json:"operations,omitempty"`
}

// Depend implements component:depend command
type Depend struct {
	action.WithLogger
	action.WithTerm

	// Arguments
	Target     string
	Operations []string

	// Show options
	Source  string
	Path    bool // show paths instead of MRNs
	Tree    bool // show tree-like output
	Reverse bool // show reverse dependencies (requiredby)
	Depth   int8 // recursion depth limit
	Build   bool // include build dependencies (from main.yaml)

	result *DependResult
}

// Result returns the structured result for JSON output.
func (d *Depend) Result() any {
	return d.result
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

	// Parse and execute operations (always filesystem-based)
	return d.executeOperations()
}

// depEdgeTypes returns dependency edge types based on the Build flag.
func (d *Depend) depEdgeTypes() []string {
	if d.Build {
		return graph.ComponentDependencyEdgeTypes()
	}
	// Exclude "builds" from dependency types
	all := graph.ComponentDependencyEdgeTypes()
	result := make([]string, 0, len(all)-1)
	for _, t := range all {
		if t != "builds" {
			result = append(result, t)
		}
	}
	return result
}

// executeShow displays dependencies using the platform graph.
func (d *Depend) executeShow() error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	// Resolve target to MRN
	searchMrn := d.Target
	if g.Node(searchMrn) == nil {
		// Not found directly — try converting from path
		c := sync.BuildComponentFromPath(d.Target, d.Source)
		if c == nil {
			return fmt.Errorf("not valid component %q", d.Target)
		}
		searchMrn = c.GetName()
		if g.Node(searchMrn) == nil {
			return fmt.Errorf("component %q not found in graph", searchMrn)
		}
	}

	edgeTypes := d.depEdgeTypes()
	depth := int(d.Depth)

	// Get parents (what depends on target) and children (what target depends on)
	ancestors := g.Ancestors(searchMrn, depth, edgeTypes...)
	descendants := g.Descendants(searchMrn, depth, edgeTypes...)

	parents := make(map[string]bool, len(ancestors))
	for _, n := range ancestors {
		parents[n.Name] = true
	}
	children := make(map[string]bool, len(descendants))
	for _, n := range descendants {
		children[n.Name] = true
	}

	// Build sorted slices for the result
	requires := make([]string, 0, len(children))
	for k := range children {
		requires = append(requires, k)
	}
	sort.Strings(requires)
	requiredBy := make([]string, 0, len(parents))
	for k := range parents {
		requiredBy = append(requiredBy, k)
	}
	sort.Strings(requiredBy)

	d.result = &DependResult{
		Target:     searchMrn,
		Mode:       "show",
		Requires:   requires,
		RequiredBy: requiredBy,
	}

	if len(parents) == 0 && len(children) == 0 {
		d.Term().Info().Println("No dependencies found")
		d.Term().Println()
		d.Term().Info().Println("Tip: DEP (add), DEP- (remove), OLD/NEW (replace)")
		return nil
	}

	if d.Tree {
		d.printTree(searchMrn, g, edgeTypes, d.Reverse, d.Path, d.Depth)
	} else if d.Reverse {
		if len(parents) > 0 {
			d.printList(parents, d.Path, "requiredby")
		}
	} else {
		if len(children) > 0 {
			d.printList(children, d.Path, "requires")
		}
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
	d.result = &DependResult{
		Target: d.Target,
		Mode:   "operations",
	}

	for _, op := range ops {
		depMrn, err := d.resolveDependencyMRN(op.Dep)
		if err != nil {
			return err
		}

		switch op.Type {
		case "add":
			applied := d.addDep(&deps, depMrn)
			d.result.Operations = append(d.result.Operations, DependOpResult{
				Type: "add", Dep: depMrn, Applied: applied,
			})
			if applied {
				d.Term().Success().Printfln("Added: %s", depMrn)
				modified = true
			} else {
				d.Term().Warning().Printfln("Already exists: %s", depMrn)
			}
		case "remove":
			applied := d.removeDep(&deps, depMrn)
			d.result.Operations = append(d.result.Operations, DependOpResult{
				Type: "remove", Dep: depMrn, Applied: applied,
			})
			if applied {
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
			applied := removed || added
			d.result.Operations = append(d.result.Operations, DependOpResult{
				Type: "replace", Dep: depMrn, NewDep: newMrn, Applied: applied,
			})
			if applied {
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
	path, err := sync.ConvertNameToPath(d.Target)
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
	if _, err := sync.ConvertNameToPath(dep); err == nil {
		return dep, nil
	}

	// Try to build component from path
	c := sync.BuildComponentFromPath(dep, ".")
	if c != nil {
		return c.GetName(), nil
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

func (d *Depend) printList(items map[string]bool, toPath bool, prefix string) {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, item := range keys {
		res := item
		if toPath {
			res, _ = sync.ConvertNameToPath(res)
		}

		d.Term().Printf("%s\t%s\n", prefix, res)
	}
}

// printTree prints a dependency tree querying the graph dynamically.
func (d *Depend) printTree(target string, g *graph.PlatformGraph, edgeTypes []string, reverse bool, toPath bool, depth int8) {
	value := target
	if toPath {
		value, _ = sync.ConvertNameToPath(value)
	}
	d.Term().Printfln(value)

	seen := make(map[string]bool)
	seen[target] = true

	d.printTreeChildren(target, g, edgeTypes, reverse, "", toPath, 0, depth, seen)
}

// printTreeChildren recursively prints tree children querying the graph.
func (d *Depend) printTreeChildren(current string, g *graph.PlatformGraph, edgeTypes []string, reverse bool, indent string, toPath bool, currentDepth, maxDepth int8, seen map[string]bool) {
	if currentDepth >= maxDepth {
		return
	}

	var childNames []string
	if reverse {
		for _, e := range g.EdgesTo(current, edgeTypes...) {
			childNames = append(childNames, e.From().Name)
		}
	} else {
		for _, e := range g.EdgesFrom(current, edgeTypes...) {
			childNames = append(childNames, e.To().Name)
		}
	}

	sort.Strings(childNames)

	for i, child := range childNames {
		isLast := i == len(childNames)-1
		edge := "├── "
		newIndent := indent + "│   "
		if isLast {
			edge = "└── "
			newIndent = indent + "    "
		}

		value := child
		if toPath {
			value, _ = sync.ConvertNameToPath(value)
		}

		if seen[child] {
			d.Term().Printfln(indent + edge + value + " [deduped]")
		} else {
			seen[child] = true
			d.Term().Printfln(indent + edge + value)
			d.printTreeChildren(child, g, edgeTypes, reverse, newIndent, toPath, currentDepth+1, maxDepth, seen)
		}
	}
}

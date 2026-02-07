package list

import (
	"fmt"
	"sort"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
)

// ComponentListItem represents a component in the list output
type ComponentListItem struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Layer   string `json:"layer"`
	Kind    string `json:"kind"`
	Chassis string `json:"chassis,omitempty"`
}

// ListResult is the structured output for component:list
type ListResult struct {
	Components []ComponentListItem `json:"components"`
}

// List implements the component:list command
type List struct {
	action.WithLogger
	action.WithTerm

	Tree    bool
	Kind    string
	All     bool
	Orphans bool

	result *ListResult
}

// Result returns the structured result for JSON output
func (l *List) Result() any {
	return l.result
}

// Execute runs the component:list action
func (l *List) Execute() error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	allNodes := g.NodesByType("component")

	var items []ComponentListItem
	for _, n := range allNodes {
		// Get chassis attachment (attaches: chassis → component)
		chassis := ""
		attachEdges := g.EdgesTo(n.Name, "attaches")
		if len(attachEdges) > 0 {
			chassis = attachEdges[0].From().Name
		}

		// Filter: attached only (default) vs all
		if !l.All && chassis == "" {
			continue
		}

		// Filter by kind
		if l.Kind != "" && n.Kind != l.Kind {
			continue
		}

		items = append(items, ComponentListItem{
			Name:    n.Name,
			Version: n.Version,
			Layer:   n.Layer,
			Kind:    n.Kind,
			Chassis: chassis,
		})
	}

	// Filter orphans if requested
	if l.Orphans {
		items = l.filterOrphans(items, g)
	}

	// Sort by name
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	if len(items) == 0 {
		l.Term().Warning().Println("No components found")
		return nil
	}

	l.result = &ListResult{Components: items}

	if l.Tree {
		return l.printTree(items, g)
	}

	// Flat output - one per line, scriptable
	for _, item := range items {
		fmt.Println(component.FormatDisplayName(item.Name, item.Version))
	}

	return nil
}

// printTree prints components as a tree with chassis paths and nodes
func (l *List) printTree(items []ComponentListItem, g *graph.PlatformGraph) error {
	// Build chassis path to nodes map from graph
	chassisToNodes := make(map[string][]string)
	for _, n := range g.NodesByType("node") {
		for _, e := range g.EdgesFrom(n.Name, "memberof") {
			chassisToNodes[e.To().Name] = append(chassisToNodes[e.To().Name], n.Name)
		}
	}

	// Sort nodes for each chassis path
	for k := range chassisToNodes {
		sort.Strings(chassisToNodes[k])
	}

	// Group components by kind
	byKind := make(map[string][]ComponentListItem)
	var kinds []string
	for _, item := range items {
		if _, ok := byKind[item.Kind]; !ok {
			kinds = append(kinds, item.Kind)
		}
		byKind[item.Kind] = append(byKind[item.Kind], item)
	}
	sort.Strings(kinds)

	for ki, kind := range kinds {
		comps := byKind[kind]
		sort.Slice(comps, func(i, j int) bool {
			return comps[i].Name < comps[j].Name
		})

		// Print kind header
		fmt.Println(kind)

		for ci, comp := range comps {
			isLastComp := ci == len(comps)-1 && ki == len(kinds)-1

			var compPrefix, compIndent string
			if isLastComp || ci == len(comps)-1 {
				compPrefix = "└── "
				compIndent = "    "
			} else {
				compPrefix = "├── "
				compIndent = "│   "
			}

			fmt.Printf("%s🧩 %s\n", compPrefix, component.FormatDisplayName(comp.Name, comp.Version))

			// Get nodes that serve this component's chassis path
			nodes := chassisToNodes[comp.Chassis]
			totalChildren := 1 + len(nodes) // chassis path + nodes

			childIdx := 0

			// Print chassis path
			childIdx++
			isLast := childIdx == totalChildren
			var childPrefix string
			if isLast {
				childPrefix = compIndent + "└── "
			} else {
				childPrefix = compIndent + "├── "
			}
			fmt.Printf("%s📍 %s\n", childPrefix, comp.Chassis)

			// Print nodes
			for _, n := range nodes {
				childIdx++
				isLast := childIdx == totalChildren
				if isLast {
					childPrefix = compIndent + "└── "
				} else {
					childPrefix = compIndent + "├── "
				}
				fmt.Printf("%s🖥  %s\n", childPrefix, n)
			}
		}

		if ki < len(kinds)-1 {
			fmt.Println()
		}
	}

	return nil
}

// topLevelKinds are component kinds that are expected to have no dependents
var topLevelKinds = map[string]bool{
	"applications": true,
	"executors":    true,
}

// filterOrphans returns components that nothing depends on, excluding top-level kinds
func (l *List) filterOrphans(items []ComponentListItem, g *graph.PlatformGraph) []ComponentListItem {
	depTypes := graph.ComponentDependencyEdgeTypes()

	var orphans []ComponentListItem
	for _, item := range items {
		if topLevelKinds[item.Kind] {
			continue
		}

		// Check if anything depends on this component
		if len(g.EdgesTo(item.Name, depTypes...)) == 0 {
			orphans = append(orphans, item)
		}
	}

	return orphans
}

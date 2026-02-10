package show

import (
	"fmt"
	"sort"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
)

// ComponentInfo represents detailed component information
type ComponentInfo struct {
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Layer       string   `json:"layer"`
	Kind        string   `json:"kind"`
	Package     string   `json:"package,omitempty"`
	Attachment  string   `json:"attachment,omitempty"`
	Allocations []string `json:"allocations,omitempty"`
}

// OverviewResult is the structured output for component:show (no args)
type OverviewResult struct {
	ByLayer  map[string]int `json:"by_layer"`
	ByKind   map[string]int `json:"by_kind"`
	Attached int            `json:"attached"`
	Total    int            `json:"total"`
}

// ShowResult is the structured output for component:show
type ShowResult struct {
	Component *ComponentInfo  `json:"component,omitempty"`
	Overview  *OverviewResult `json:"overview,omitempty"`
}

// Show implements the component:show command
type Show struct {
	action.WithLogger
	action.WithTerm

	Component string

	result *ShowResult
}

// Result returns the structured result for JSON output
func (s *Show) Result() any {
	return s.result
}

// Execute runs the component:show action
func (s *Show) Execute() error {
	// If no component specified, show overview
	if s.Component == "" {
		return s.showOverview()
	}

	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	n := g.Node(s.Component)
	if n == nil || n.Type != "component" {
		s.Term().Error().Printfln("Component %q not found", s.Component)
		return nil
	}

	// Get chassis attachment (attaches: chassis → component)
	chassis := ""
	attachEdges := g.EdgesTo(n.Name, "distributes")
	if len(attachEdges) > 0 {
		chassis = attachEdges[0].From().Name
	}

	// Get package (package → component via "contains" edge)
	pkg := ""
	containsEdges := g.EdgesTo(n.Name, "contains")
	for _, e := range containsEdges {
		if e.From().Type == "package" {
			pkg = e.From().Name
			break
		}
	}

	// Get allocations (nodes serving the chassis path)
	var allocations []string
	if chassis != "" {
		for _, e := range g.EdgesTo(chassis, "allocates") {
			allocations = append(allocations, e.From().Name)
		}
		sort.Strings(allocations)
	}

	// Build result
	s.result = &ShowResult{
		Component: &ComponentInfo{
			Name:        n.Name,
			Version:     n.Version,
			Layer:       n.Layer,
			Kind:        n.Kind,
			Package:     pkg,
			Attachment:  chassis,
			Allocations: allocations,
		},
	}

	// Print human-readable output
	s.printComponent(s.result.Component)

	return nil
}

// showOverview displays component statistics grouped by layer and kind
func (s *Show) showOverview() error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	allComponents := g.NodesByType("component")

	byLayer := make(map[string]int)
	byKind := make(map[string]int)
	attachedByKind := make(map[string]int)
	attachedCount := 0

	for _, n := range allComponents {
		byLayer[n.Layer]++
		byKind[n.Kind]++

		attachEdges := g.EdgesTo(n.Name, "distributes")
		if len(attachEdges) > 0 {
			attachedCount++
			attachedByKind[n.Kind]++
		}
	}

	// Build result
	s.result = &ShowResult{
		Overview: &OverviewResult{
			ByLayer:  byLayer,
			ByKind:   byKind,
			Attached: attachedCount,
			Total:    len(allComponents),
		},
	}

	// Print by layer
	s.Term().Info().Printfln("By Layer (%d total)", len(allComponents))
	layers := sortedKeys(byLayer)
	for _, layer := range layers {
		s.Term().Printfln("  %s\t%d", layer, byLayer[layer])
	}

	// Print by kind
	s.Term().Info().Println("By Kind")
	kinds := sortedKeys(byKind)
	for _, kind := range kinds {
		attached := attachedByKind[kind]
		if attached > 0 {
			s.Term().Printfln("  %s\t%d (%d attached)", kind, byKind[kind], attached)
		} else {
			s.Term().Printfln("  %s\t%d", kind, byKind[kind])
		}
	}

	// Print summary
	s.Term().Info().Printfln("Attached: %d components", attachedCount)
	s.Term().Info().Printfln("Total: %d components", len(allComponents))

	return nil
}

// sortedKeys returns map keys sorted alphabetically
func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// printComponent outputs human-readable component details
func (s *Show) printComponent(comp *ComponentInfo) {
	s.Term().Printfln("component\t%s", comp.Name)
	s.Term().Printfln("version\t%s", component.FormatVersion(comp.Version))
	s.Term().Printfln("layer\t%s", comp.Layer)
	s.Term().Printfln("kind\t%s", comp.Kind)
	if comp.Package != "" {
		s.Term().Printfln("package\t%s", comp.Package)
	}
	if comp.Attachment != "" {
		s.Term().Printfln("attachment\t%s", comp.Attachment)
	} else {
		s.Term().Printfln("attachment\t(not attached)")
	}

	if len(comp.Allocations) > 0 {
		s.Term().Info().Printfln("Allocations (%d)", len(comp.Allocations))
		for _, n := range comp.Allocations {
			s.Term().Printfln("%s", n)
		}
	}
}

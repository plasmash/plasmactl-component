package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/pkg/component"
	"github.com/plasmash/plasmactl-platform/pkg/graph"
)

// ComponentMatch represents a component found by query
type ComponentMatch struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Zone    string `json:"zone"`
}

// QueryResult is the structured output for component:query
type QueryResult struct {
	Components []ComponentMatch `json:"components"`
}

// Query implements the component:query command
type Query struct {
	action.WithLogger
	action.WithTerm

	Identifier string
	Kind       string // "zone" or "node" to skip auto-detection

	result QueryResult
}

// Execute runs the query action
func (q *Query) Execute() error {
	g, err := graph.Load()
	if err != nil {
		return fmt.Errorf("failed to load graph: %w", err)
	}

	var matches []componentMatch

	searchZone := q.Kind == "" || q.Kind == "zone"
	searchNode := q.Kind == "" || q.Kind == "node"

	// Try 1: Query by zone (distributes: zone → component)
	if searchZone {
		for _, n := range g.NodesByType("component") {
			for _, e := range g.EdgesTo(n.Name, "distributes") {
				zone := e.From().Name
				if zone == q.Identifier || strings.HasPrefix(zone, q.Identifier+".") {
					matches = append(matches, componentMatch{
						name:    n.Name,
						version: n.Version,
						kind:    n.Kind,
						zone:    zone,
					})
				}
			}
		}
	}

	// Try 2: Query by node hostname
	if searchNode && len(matches) == 0 {
		nodeNode := g.Node(q.Identifier)
		if nodeNode != nil && nodeNode.Type == "node" {
			// Get zones this node serves
			zoneSet := make(map[string]bool)
			for _, e := range g.EdgesFrom(nodeNode.Name, "allocates") {
				zoneSet[e.To().Name] = true
			}

			// Find components attached to those zones (distributes: zone → component)
			for _, n := range g.NodesByType("component") {
				for _, e := range g.EdgesTo(n.Name, "distributes") {
					if zoneSet[e.From().Name] {
						matches = append(matches, componentMatch{
							name:    n.Name,
							version: n.Version,
							kind:    n.Kind,
							zone:    e.From().Name,
						})
					}
				}
			}
		}
	}

	if len(matches) == 0 {
		q.Term().Warning().Printfln("No components found for %q", q.Identifier)
		return nil
	}

	// Sort by kind, then name
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].kind != matches[j].kind {
			return matches[i].kind < matches[j].kind
		}
		return matches[i].name < matches[j].name
	})

	// Build result
	for _, m := range matches {
		q.result.Components = append(q.result.Components, ComponentMatch{
			Name:    m.name,
			Version: m.version,
			Kind:    m.kind,
			Zone:    m.zone,
		})
	}

	// Output
	for _, m := range matches {
		q.Term().Printfln("%s\t%s\t%s", m.DisplayName(), m.kind, m.zone)
	}

	return nil
}

// Result returns the structured result for JSON output
func (q *Query) Result() any {
	return q.result
}

type componentMatch struct {
	name    string
	version string
	kind    string
	zone    string
}

// DisplayName returns the component formatted as "name@version".
func (m componentMatch) DisplayName() string {
	return component.FormatDisplayName(m.name, m.version)
}

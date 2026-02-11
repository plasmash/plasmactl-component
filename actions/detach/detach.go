package detach

import (
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/playbook"
)

// DetachResult is the structured result of component:detach.
type DetachResult struct {
	Component string `json:"component"`
	Chassis   string `json:"chassis"`
	Detached  bool   `json:"detached"`
}

// Detach implements component:detach command
type Detach struct {
	action.WithLogger
	action.WithTerm

	Component string
	Chassis   string
	Source    string

	result *DetachResult
}

// Result returns the structured result for JSON output.
func (d *Detach) Result() any {
	return d.result
}

// Execute runs the detach action
func (d *Detach) Execute() error {
	layer := playbook.ExtractLayer(d.Component)
	if layer == "" {
		return fmt.Errorf("invalid component MRN %q: cannot extract layer", d.Component)
	}

	playbookPath, err := playbook.FindPlaybook(d.Source, layer)
	if err != nil {
		return err
	}

	plays, err := playbook.Load(playbookPath)
	if err != nil {
		return err
	}

	plays, detached := playbook.RemoveRole(plays, d.Component, d.Chassis)
	if !detached {
		d.result = &DetachResult{Component: d.Component, Chassis: d.Chassis, Detached: false}
		d.Term().Warning().Printfln("Component %s not attached to %s", d.Component, d.Chassis)
		return nil
	}

	if err := playbook.Save(playbookPath, plays); err != nil {
		return err
	}

	d.result = &DetachResult{Component: d.Component, Chassis: d.Chassis, Detached: true}
	d.Term().Success().Printfln("Detached %s from %s", d.Component, d.Chassis)
	return nil
}

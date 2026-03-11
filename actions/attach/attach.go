package attach

import (
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/playbook"
)

// AttachResult is the structured result of component:attach.
type AttachResult struct {
	Component string `json:"component"`
	Zone      string `json:"zone"`
	Attached  bool   `json:"attached"`
}

// Attach implements component:attach command
type Attach struct {
	action.WithLogger
	action.WithTerm

	Component string
	Zone      string
	Source    string

	result *AttachResult
}

// Result returns the structured result for JSON output.
func (a *Attach) Result() any {
	return a.result
}

// Execute runs the attach action
func (a *Attach) Execute() error {
	layer := playbook.ExtractLayer(a.Component)
	if layer == "" {
		return fmt.Errorf("invalid component MRN %q: cannot extract layer", a.Component)
	}

	playbookPath, err := playbook.FindPlaybook(a.Source, layer)
	if err != nil {
		return err
	}

	plays, err := playbook.Load(playbookPath)
	if err != nil {
		return err
	}

	plays, attached := playbook.AddRole(plays, a.Component, a.Zone)
	if !attached {
		a.result = &AttachResult{Component: a.Component, Zone: a.Zone, Attached: false}
		a.Term().Warning().Printfln("Component %s already attached to %s", a.Component, a.Zone)
		return nil
	}

	if err := playbook.Save(playbookPath, plays); err != nil {
		return err
	}

	a.result = &AttachResult{Component: a.Component, Zone: a.Zone, Attached: true}
	a.Term().Success().Printfln("Attached %s to %s", a.Component, a.Zone)
	return nil
}

package attach

import (
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/playbook"
)

// Attach implements component:attach command
type Attach struct {
	action.WithLogger
	action.WithTerm

	Component string
	Chassis   string
	Source    string
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

	plays, attached := playbook.AddRole(plays, a.Component, a.Chassis)
	if !attached {
		a.Term().Warning().Printfln("Component %s already attached to %s", a.Component, a.Chassis)
		return nil
	}

	if err := playbook.Save(playbookPath, plays); err != nil {
		return err
	}

	a.Term().Success().Printfln("Attached %s to %s", a.Component, a.Chassis)
	return nil
}

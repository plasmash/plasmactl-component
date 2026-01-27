package detach

import (
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/playbook"
)

// Detach implements component:detach command
type Detach struct {
	action.WithLogger
	action.WithTerm

	Component string
	Chassis   string
	Source    string
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
		d.Term().Warning().Printfln("Component %s not attached to %s", d.Component, d.Chassis)
		return nil
	}

	if err := playbook.Save(playbookPath, plays); err != nil {
		return err
	}

	d.Term().Success().Printfln("Detached %s from %s", d.Component, d.Chassis)
	return nil
}

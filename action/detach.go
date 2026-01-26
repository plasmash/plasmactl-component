package action

import (
	"fmt"

	"github.com/launchrctl/launchr/pkg/action"
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
	layer := extractLayer(d.Component)
	if layer == "" {
		return fmt.Errorf("invalid component MRN %q: cannot extract layer", d.Component)
	}

	playbookPath, err := findPlaybook(d.Source, layer)
	if err != nil {
		return err
	}

	plays, err := loadPlaybook(playbookPath)
	if err != nil {
		return err
	}

	plays, detached := removeFromPlay(plays, d.Component, d.Chassis)
	if !detached {
		d.Term().Warning().Printfln("Component %s not attached to %s", d.Component, d.Chassis)
		return nil
	}

	if err := savePlaybook(playbookPath, plays); err != nil {
		return err
	}

	d.Term().Success().Printfln("Detached %s from %s", d.Component, d.Chassis)
	return nil
}

package bump

import (
	"path/filepath"
	"strings"

	"github.com/launchrctl/launchr/pkg/action"
	"github.com/plasmash/plasmactl-component/internal/repository"
	"github.com/plasmash/plasmactl-component/internal/sync"
)

var unversionedFiles = map[string]struct{}{
	"README.md":  {},
	"README.svg": {},
}

// Bump is an action representing versions update of committed components.
type Bump struct {
	action.WithLogger
	action.WithTerm

	Last   bool
	DryRun bool
}

func (b *Bump) printMemo() {
	b.Log().Info("List of non-versioned files:")
	for k := range unversionedFiles {
		b.Log().Info(k)
	}
}

// Execute the bump action to update committed components.
func (b *Bump) Execute() error {
	b.Term().Info().Println("Bumping updated components...")
	b.printMemo()

	bumper, err := repository.NewBumper()
	if err != nil {
		return err
	}

	if bumper.IsOwnCommit() {
		b.Term().Info().Println("skipping bump, as the latest commit is already by the bumper tool")
		return nil
	}

	commits, err := bumper.GetCommits(b.Last)
	if err != nil {
		return err
	}

	components := b.collectComponents(commits)
	if len(components) == 0 {
		b.Term().Info().Println("No component to update")
		return nil
	}

	err = b.updateComponents(components)
	if err != nil {
		b.Log().Error("There is an error during components update")
		return err
	}

	if b.DryRun {
		return nil
	}

	return bumper.Commit()
}

func (b *Bump) getComponent(path string) *sync.Component {
	if !isVersionableFile(path) {
		return nil
	}

	platform, kind, role, err := sync.ProcessComponentPath(path)
	if err != nil || (platform == "" || kind == "" || role == "") {
		return nil
	}

	// skip actions dir from triggering bump.
	componentActionsDir := filepath.Join(platform, kind, "roles", role, "actions")
	if strings.Contains(path, componentActionsDir) {
		return nil
	}

	component := sync.NewComponent(sync.PrepareComponentName(platform, kind, role), ".")
	if !component.IsValidComponent() {
		return nil
	}

	return component
}

func (b *Bump) collectComponents(commits []*repository.Commit) map[string]map[string]*sync.Component {
	uniqueVersion := map[string]string{}

	components := make(map[string]map[string]*sync.Component)
	for _, c := range commits {
		hash := c.Hash[:13]
		for _, path := range c.Files {
			component := b.getComponent(path)
			if component == nil {
				continue
			}

			if _, ok := components[hash]; !ok {
				components[hash] = make(map[string]*sync.Component)
			}

			if _, ok := uniqueVersion[component.GetName()]; ok {
				continue
			}

			b.Term().Printfln("Processing component %s", component.GetName())
			components[hash][component.GetName()] = component
			uniqueVersion[component.GetName()] = hash
		}
	}

	return components
}

func (b *Bump) updateComponents(hashComponentsMap map[string]map[string]*sync.Component) error {
	if len(hashComponentsMap) == 0 {
		return nil
	}

	b.Term().Printf("Updating versions:\n")
	for version, components := range hashComponentsMap {
		for name, c := range components {
			currentVersion, debug, err := c.GetVersion()
			for _, d := range debug {
				b.Log().Debug("error", "message", d)
			}
			if err != nil {
				return err
			}

			b.Term().Printfln("- %s from %s to %s", name, currentVersion, version)
			if b.DryRun {
				continue
			}

			debug, err = c.UpdateVersion(version)
			for _, d := range debug {
				b.Log().Debug("error", "message", d)
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func isVersionableFile(path string) bool {
	name := filepath.Base(path)
	_, ok := unversionedFiles[name]
	return !ok
}

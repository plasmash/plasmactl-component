package sync

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	async "sync"
	"time"

	"github.com/launchrctl/compose/compose"
	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
	"github.com/pterm/pterm"

	"github.com/plasmash/plasmactl-component/internal/sync"
)

var (
	errMalformedKeyring = errors.New("the keyring is malformed or wrong passphrase provided")
)

const (
	vaultpassKey    = "vaultpass"
	domainNamespace = "domain"
	buildHackAuthor = "override"
)

// Sync is a type representing a components version synchronization action.
type Sync struct {
	action.WithLogger
	action.WithTerm

	// services.
	Keyring keyring.Keyring
	Streams launchr.Streams

	// target dirs.
	BuildDir    string
	PackagesDir string
	DomainDir   string

	// internal.
	saveKeyring bool
	timeline    []sync.TimelineItem

	// options.
	DryRun                 bool
	AllowOverride          bool
	FilterByComponentUsage bool
	TimeDepth              string
	VaultPass              string
	ShowProgress           bool
}

type hashStruct struct {
	hash     string
	hashTime time.Time
	author   string
}

// Execute the sync action to propagate resources' versions.
func (s *Sync) Execute() error {
	s.Term().Info().Println("Processing propagation...")

	err := s.ensureVaultpassExists()
	if err != nil {
		return err
	}

	err = s.propagate()
	if err != nil {
		return err
	}
	s.Term().Info().Println("Propagation has been finished")

	if s.saveKeyring {
		err = s.Keyring.Save()
	}

	return err
}

func (s *Sync) ensureVaultpassExists() error {
	keyValueItem, errGet := s.Keyring.GetForKey(vaultpassKey)
	if errGet != nil {
		if errors.Is(errGet, keyring.ErrEmptyPass) {
			return errGet
		} else if !errors.Is(errGet, keyring.ErrNotFound) {
			s.Log().Debug("keyring error", "error", errGet)
			return errMalformedKeyring
		}

		keyValueItem.Key = vaultpassKey
		keyValueItem.Value = s.VaultPass

		if keyValueItem.Value == "" {
			s.Term().Printf("- Ansible vault password\n")
			err := keyring.RequestKeyValueFromTty(&keyValueItem)
			if err != nil {
				return err
			}
		}

		err := s.Keyring.AddItem(keyValueItem)
		if err != nil {
			return err
		}
		s.saveKeyring = true
	}

	s.VaultPass = keyValueItem.Value.(string)

	return nil
}

func (s *Sync) propagate() error {
	s.timeline = sync.CreateTimeline()

	s.Log().Info("Initializing build inventory")
	inv, err := sync.NewInventory(s.BuildDir, s.Log())
	if err != nil {
		return err
	}

	if s.FilterByComponentUsage {
		s.Log().Info("Calculating components usage")
		err = inv.CalculateComponentsUsage()
		if err != nil {
			return fmt.Errorf("calculate components usage > %w", err)
		}
	}

	s.Log().Info("Calculating variables usage")
	err = inv.CalculateVariablesUsage(s.VaultPass)
	if err != nil {
		return fmt.Errorf("calculate variables usage > %w", err)
	}

	err = s.buildTimeline(inv)
	if err != nil {
		return fmt.Errorf("building timeline > %w", err)
	}

	if len(s.timeline) == 0 {
		s.Term().Warning().Println("No components were found for propagation")
		return nil
	}

	toSync, componentVersionMap, err := s.buildPropagationMap(inv, s.timeline)
	if err != nil {
		return fmt.Errorf("building propagation map > %w", err)
	}

	err = s.updateComponents(componentVersionMap, toSync)
	if err != nil {
		return fmt.Errorf("propagate > %w", err)
	}

	return nil
}

func (s *Sync) buildTimeline(buildInv *sync.Inventory) error {
	s.Log().Info("Gathering domain and packages components")
	componentsMap, packagePathMap, err := s.getComponentsMaps(buildInv)
	if err != nil {
		return fmt.Errorf("build component map > %w", err)
	}

	s.Log().Info("Populate timeline with components")
	err = s.populateTimelineComponents(componentsMap, packagePathMap)
	if err != nil {
		return fmt.Errorf("iterating components > %w", err)
	}

	s.Log().Info("Populate timeline with variables")
	err = s.populateTimelineVars(buildInv)
	if err != nil {
		return fmt.Errorf("iteraring variables > %w", err)
	}

	return nil
}

func (s *Sync) getComponentsMaps(buildInv *sync.Inventory) (map[string]*sync.OrderedMap[*sync.Component], map[string]string, error) {
	componentsMap := make(map[string]*sync.OrderedMap[*sync.Component])
	packagePathMap := make(map[string]string)

	plasmaCompose, err := compose.Lookup(os.DirFS(s.DomainDir))
	if err != nil {
		return nil, nil, err
	}

	var priorityOrder []string
	for _, dep := range plasmaCompose.Dependencies {
		pkg := dep.ToPackage(dep.Name)
		packagePathMap[dep.Name] = filepath.Join(s.PackagesDir, pkg.GetName(), pkg.GetTarget())
		priorityOrder = append(priorityOrder, dep.Name)
	}

	packagePathMap[domainNamespace] = s.DomainDir

	priorityOrder = append(priorityOrder, domainNamespace)

	var wg async.WaitGroup
	var mx async.Mutex

	maxWorkers := min(runtime.NumCPU(), len(packagePathMap))
	workChan := make(chan map[string]string, len(packagePathMap))
	errorChan := make(chan error, 1)

	for i := 0; i < maxWorkers; i++ {
		go func() {
			for repo := range workChan {
				components, errRes := s.getComponentsMapFrom(repo["path"])
				if errRes != nil {
					errorChan <- errRes
					return
				}

				mx.Lock()
				componentsMap[repo["package"]] = components
				mx.Unlock()
				wg.Done()
			}
		}()
	}

	for pkg, path := range packagePathMap {
		wg.Add(1)
		workChan <- map[string]string{"path": path, "package": pkg}
	}

	close(workChan)

	go func() {
		wg.Wait()
		close(errorChan)
	}()

	for err = range errorChan {
		if err != nil {
			return nil, nil, err
		}
	}

	// Remove unused components from packages maps.
	if s.FilterByComponentUsage {
		usedComponents := buildInv.GetUsedComponents()
		if len(usedComponents) == 0 {
			// Empty maps and return, as no components are used in build.
			componentsMap = make(map[string]*sync.OrderedMap[*sync.Component])
			packagePathMap = make(map[string]string)

			return componentsMap, packagePathMap, nil
		}

		s.Log().Info("List of used components:")
		var uc []string
		for c := range usedComponents {
			uc = append(uc, c)
		}

		sort.Strings(uc)
		for _, c := range uc {
			s.Log().Info(fmt.Sprintf("- %s", c))
		}

		s.Log().Info("List of unused components:")
		for p, components := range componentsMap {
			s.Log().Info(fmt.Sprintf("- Package - %s -", p))
			for _, k := range components.Keys() {
				if _, ok := usedComponents[k]; !ok {
					s.Log().Info(fmt.Sprintf("- %s", k))
					components.Unset(k)
				}
			}
		}
	}

	buildComponents := buildInv.GetComponentsMap()
	for _, componentName := range buildComponents.Keys() {
		conflicts := make(map[string]string)
		for name, components := range componentsMap {
			if _, ok := components.Get(componentName); ok {
				conflicts[name] = ""
			}
		}

		if len(conflicts) < 2 {
			continue
		}

		buildComponentEntity := sync.NewComponent(componentName, s.BuildDir)
		buildVersion, debug, err := buildComponentEntity.GetVersion()
		for _, d := range debug {
			s.Log().Debug("error", "message", d)
		}
		if err != nil {
			return nil, nil, err
		}

		var sameVersionNamespaces []string
		for conflictingNamespace := range conflicts {
			conflictEntity := sync.NewComponent(componentName, packagePathMap[conflictingNamespace])

			baseVersion, _, debug, err := conflictEntity.GetBaseVersion()
			for _, d := range debug {
				s.Log().Debug("error", "message", d)
			}

			if err != nil {
				return nil, nil, err
			}

			if baseVersion != buildVersion {
				s.Log().Debug("removing component from namespace because of composition strategy",
					"component", componentName, "version", baseVersion, "buildVersion", buildVersion, "namespace", conflictingNamespace)
				componentsMap[conflictingNamespace].Unset(componentName)
			} else {
				sameVersionNamespaces = append(sameVersionNamespaces, conflictingNamespace)
			}
		}

		if len(sameVersionNamespaces) > 1 {
			s.Log().Debug("resolving additional strategies conflict for component", "component", componentName)
			var highest string
			for i := len(priorityOrder) - 1; i >= 0; i-- {
				if _, ok := componentsMap[priorityOrder[i]]; ok {
					highest = priorityOrder[i]
					break
				}
			}

			for i := len(priorityOrder) - 1; i >= 0; i-- {
				if priorityOrder[i] != highest {
					if _, ok := componentsMap[priorityOrder[i]]; ok {
						componentsMap[priorityOrder[i]].Unset(componentName)
					}
				}
			}
		}
	}

	return componentsMap, packagePathMap, nil
}

func (s *Sync) buildPropagationMap(buildInv *sync.Inventory, timeline []sync.TimelineItem) (*sync.OrderedMap[*sync.Component], map[string]string, error) {
	componentVersionMap := make(map[string]string)
	toSync := sync.NewOrderedMap[*sync.Component]()
	componentsMap := buildInv.GetComponentsMap()
	processed := make(map[string]bool)
	sync.SortTimeline(timeline, sync.SortDesc)

	usedComponents := make(map[string]bool)
	if s.FilterByComponentUsage {
		usedComponents = buildInv.GetUsedComponents()
	}

	s.Log().Info("Iterating timeline")
	for _, item := range timeline {
		dependenciesLog := sync.NewOrderedMap[bool]()

		switch i := item.(type) {
		case *sync.TimelineComponentsItem:
			components := i.GetComponents()
			components.SortKeysAlphabetically()

			var toProcess []string
			for _, key := range components.Keys() {
				// Skip component if it was processed by previous timeline item or previous component (via deps).
				if processed[key] {
					continue
				}

				c, _ := components.Get(key)

				if !sync.IsUpdatableKind(c.GetKind()) {
					s.Log().Warn(fmt.Sprintf("%s is not allowed to propagate", key))
					continue
				}

				toProcess = append(toProcess, key)
			}

			if len(toProcess) == 0 {
				continue
			}

			for _, key := range toProcess {
				c, ok := components.Get(key)
				if !ok {
					return nil, nil, fmt.Errorf("unknown key %s detected during timeline iteration", key)
				}

				processed[key] = true

				dependentComponents := buildInv.GetRequiredByComponents(c.GetName(), -1)
				if s.FilterByComponentUsage {
					for dc := range dependentComponents {
						if _, okU := usedComponents[dc]; !okU {
							delete(dependentComponents, dc)
						}
					}
				}

				for dep := range dependentComponents {
					depComponent, okC := componentsMap.Get(dep)
					if !okC {
						continue
					}

					// Skip component if it was processed by previous timeline item or previous component (via deps).
					if processed[dep] {
						continue
					}

					processed[dep] = true

					if !sync.IsUpdatableKind(depComponent.GetKind()) {
						s.Log().Warn(fmt.Sprintf("%s is not allowed to propagate", dep))
						continue
					}

					toSync.Set(dep, depComponent)
					componentVersionMap[dep] = i.GetVersion()

					if _, okD := components.Get(dep); !okD {
						dependenciesLog.Set(dep, true)
					}
				}
			}

			for _, key := range toProcess {
				// Ensure new version removes previous propagation for that component.
				toSync.Unset(key)
				delete(componentVersionMap, key)
			}

			if dependenciesLog.Len() > 0 {
				s.Log().Debug("timeline item (components)",
					slog.String("version", item.GetVersion()),
					slog.Time("date", item.GetDate()),
					slog.String("components", fmt.Sprintf("%v", toProcess)),
					slog.String("dependencies", fmt.Sprintf("%v", dependenciesLog.Keys())),
				)
			}

		case *sync.TimelineVariablesItem:
			variables := i.GetVariables()
			variables.SortKeysAlphabetically()

			var components []string
			for _, v := range variables.Keys() {
				variable, _ := variables.Get(v)
				vc := buildInv.GetVariableComponents(variable.GetName(), variable.GetPlatform())

				if len(usedComponents) == 0 {
					components = append(components, vc...)
				} else {
					var usedVc []string
					for _, c := range vc {
						if _, ok := usedComponents[c]; ok {
							usedVc = append(usedVc, c)
						}
					}
					components = append(components, usedVc...)
				}
			}

			slices.Sort(components)
			components = slices.Compact(components)

			var toProcess []string
			for _, key := range components {
				if processed[key] {
					continue
				}
				toProcess = append(toProcess, key)
			}

			if len(toProcess) == 0 {
				continue
			}

			for _, c := range toProcess {
				// First set version for main component.
				mainComponent, okM := componentsMap.Get(c)
				if !okM {
					s.Log().Warn(fmt.Sprintf("skipping not valid component %s (direct vars dependency)", c))
					continue
				}

				processed[c] = true

				if sync.IsUpdatableKind(mainComponent.GetKind()) {
					toSync.Set(c, mainComponent)
					componentVersionMap[c] = i.GetVersion()
					dependenciesLog.Set(c, true)
				}

				// Set versions for dependent components.
				dependentComponents := buildInv.GetRequiredByComponents(c, -1)
				for dep := range dependentComponents {
					depComponent, okC := componentsMap.Get(dep)
					if !okC {
						s.Log().Warn(fmt.Sprintf("skipping not valid component %s (dependency of %s)", dep, c))
						continue
					}

					// Skip component if it was processed by previous timeline item or previous component (via deps).
					if processed[dep] {
						continue
					}

					processed[dep] = true

					if !sync.IsUpdatableKind(depComponent.GetKind()) {
						s.Log().Warn(fmt.Sprintf("%s is not allowed to propagate", dep))
						continue
					}

					toSync.Set(dep, depComponent)
					componentVersionMap[dep] = i.GetVersion()

					dependenciesLog.Set(dep, true)
				}
			}

			if dependenciesLog.Len() > 0 {
				s.Log().Debug("timeline item (variables)",
					slog.String("version", item.GetVersion()),
					slog.Time("date", item.GetDate()),
					slog.String("variables", fmt.Sprintf("%v", variables.Keys())),
					slog.String("components", fmt.Sprintf("%v", dependenciesLog.Keys())),
				)
			}
		}
	}

	return toSync, componentVersionMap, nil
}

func (s *Sync) updateComponents(componentVersionMap map[string]string, toSync *sync.OrderedMap[*sync.Component]) error {
	var sortList []string
	updateMap := make(map[string]map[string]string)
	stopPropagation := false

	s.Log().Info("Sorting components before update")
	for _, key := range toSync.Keys() {
		c, _ := toSync.Get(key)
		baseVersion, currentVersion, debug, errVersion := c.GetBaseVersion()
		for _, d := range debug {
			s.Log().Debug("error", "message", d)
		}
		if errVersion != nil {
			return errVersion
		}

		if currentVersion == "" {
			s.Term().Warning().Printfln("component %s has no version", c.GetName())
			stopPropagation = true
		}

		newVersion := composeVersion(currentVersion, componentVersionMap[c.GetName()])
		if baseVersion == componentVersionMap[c.GetName()] {
			s.Log().Debug("skip identical",
				"baseVersion", baseVersion, "currentVersion", currentVersion, "propagateVersion", componentVersionMap[c.GetName()], "newVersion", newVersion)
			s.Term().Warning().Printfln("- skip %s (identical versions)", c.GetName())
			continue
		}

		if _, ok := updateMap[c.GetName()]; !ok {
			updateMap[c.GetName()] = make(map[string]string)
		}

		updateMap[c.GetName()]["new"] = newVersion
		updateMap[c.GetName()]["current"] = currentVersion
		sortList = append(sortList, c.GetName())
	}

	if stopPropagation {
		return errors.New("empty version has been detected, please check log")
	}

	if len(updateMap) == 0 {
		s.Term().Printfln("No version to propagate")
		return nil
	}

	sort.Strings(sortList)
	s.Log().Info("Propagating versions")

	var p *pterm.ProgressbarPrinter
	if s.ShowProgress {
		p, _ = pterm.DefaultProgressbar.WithWriter(s.Term()).WithTotal(len(sortList)).WithTitle("Updating components").Start()
	}
	for _, key := range sortList {
		if p != nil {
			p.Increment()
		}

		val := updateMap[key]

		c, ok := toSync.Get(key)
		currentVersion := val["current"]
		newVersion := val["new"]
		if !ok {
			return fmt.Errorf("unidentified component found during update %s", key)
		}

		s.Log().Info(fmt.Sprintf("%s from %s to %s", c.GetName(), currentVersion, newVersion))
		if s.DryRun {
			continue
		}

		debug, err := c.UpdateVersion(newVersion)
		for _, d := range debug {
			s.Log().Debug("error", "message", d)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func composeVersion(oldVersion string, newVersion string) string {
	var version string
	if len(strings.Split(newVersion, "-")) > 1 {
		version = newVersion
	} else {
		split := strings.Split(oldVersion, "-")
		if len(split) == 1 {
			version = fmt.Sprintf("%s-%s", oldVersion, newVersion)
		} else if len(split) > 1 {
			version = fmt.Sprintf("%s-%s", split[0], newVersion)
		} else {
			version = newVersion
		}
	}

	return version
}

func (s *Sync) getComponentsMapFrom(dir string) (*sync.OrderedMap[*sync.Component], error) {
	inv, err := sync.NewInventory(dir, s.Log())
	if err != nil {
		return nil, err
	}

	cm := inv.GetComponentsMap()
	cm.SortKeysAlphabetically()
	return cm, nil
}

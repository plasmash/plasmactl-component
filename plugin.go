// Package plasmactlcomponent implements a launchr plugin with component functionality
package plasmactlcomponent

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
)

//go:embed action.bump.yaml
var actionBumpYaml []byte

//go:embed action.sync.yaml
var actionSyncYaml []byte

//go:embed action.dependencies.yaml
var actionDependenciesYaml []byte

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] plugin providing bump functionality.
type Plugin struct {
	k   keyring.Keyring
	cfg launchr.Config
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 10,
	}
}

// OnAppInit implements [launchr.Plugin] interface.
func (p *Plugin) OnAppInit(app launchr.App) error {
	app.Services().Get(&p.cfg)
	app.Services().Get(&p.k)
	return nil
}

// DiscoverActions implements [launchr.ActionDiscoveryPlugin] interface.
func (p *Plugin) DiscoverActions(_ context.Context) ([]*action.Action, error) {
	ba := action.NewFromYAML("component:bump", actionBumpYaml)
	ba.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		dryRun := input.Opt("dry-run").(bool)
		last := input.Opt("last").(bool)

		log, _, _, term := getLogger(a)

		bump := bumpAction{last: last, dryRun: dryRun}
		bump.SetLogger(log)
		bump.SetTerm(term)
		return bump.Execute()
	}))

	sa := action.NewFromYAML("component:sync", actionSyncYaml)
	sa.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		dryRun := input.Opt("dry-run").(bool)
		allowOverride := input.Opt("allow-override").(bool)
		filterByResourceUsage := input.Opt("playbook-filter").(bool)
		timeDepth := input.Opt("time-depth").(string)
		vaultpass := input.Opt("vault-pass").(string)

		log, logLevel, streams, term := getLogger(a)
		hideProgress := input.Opt("hide-progress").(bool)
		if logLevel > 0 {
			hideProgress = true
		}

		sync := syncAction{
			keyring: p.k,
			streams: streams,

			domainDir:   ".",
			buildDir:    ".plasma/package/compose/merged",
			packagesDir: ".plasma/package/compose/packages",

			dryRun:                dryRun,
			filterByResourceUsage: filterByResourceUsage,
			timeDepth:             timeDepth,
			allowOverride:         allowOverride,
			vaultPass:             vaultpass,
			showProgress:          !hideProgress,
		}

		sync.SetLogger(log)
		sync.SetTerm(term)
		return sync.Execute()
	}))

	da := action.NewFromYAML("component:depend", actionDependenciesYaml)
	da.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)

		input := a.Input()
		source := input.Opt("source").(string)
		if _, err := os.Stat(source); os.IsNotExist(err) {
			term.Warning().Printfln("%s doesn't exist, fallback to current dir", source)
			source = "."
		} else {
			term.Info().Printfln("Selected source is %s", source)
		}

		showPaths := input.Opt("mrn").(bool)
		showTree := input.Opt("tree").(bool)
		depth := int8(input.Opt("depth").(int)) //nolint:gosec
		if depth == 0 {
			return fmt.Errorf("depth value should not be zero")
		}

		target := input.Arg("target").(string)
		dependencies := &dependenciesAction{}
		dependencies.SetLogger(log)
		dependencies.SetTerm(term)
		return dependencies.run(target, source, !showPaths, showTree, depth)
	}))

	return []*action.Action{ba, sa, da}, nil
}

func getLogger(a *action.Action) (*launchr.Logger, launchr.LogLevel, launchr.Streams, *launchr.Terminal) {
	log := launchr.Log()
	level := log.Level()
	if rt, ok := a.Runtime().(action.RuntimeLoggerAware); ok {
		log = rt.LogWith()
		level = log.Level()
	}

	term := launchr.Term()
	if rt, ok := a.Runtime().(action.RuntimeTermAware); ok {
		term = rt.Term()
	}

	return log, level, a.Input().Streams(), term
}

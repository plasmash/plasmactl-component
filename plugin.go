// Package plasmactlcomponent implements a launchr plugin with component functionality
package plasmactlcomponent

import (
	"context"
	"embed"
	"fmt"
	"os"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"
	caction "github.com/plasmash/plasmactl-component/action"
)

//go:embed action/*.yaml
var actionYamlFS embed.FS

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
	// component:bump action
	actionBumpYaml, _ := actionYamlFS.ReadFile("action/bump.yaml")
	ba := action.NewFromYAML("component:bump", actionBumpYaml)
	ba.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		dryRun := input.Opt("dry-run").(bool)
		last := input.Opt("last").(bool)

		log, _, _, term := getLogger(a)

		bump := caction.Bump{Last: last, DryRun: dryRun}
		bump.SetLogger(log)
		bump.SetTerm(term)
		return bump.Execute()
	}))

	// component:sync action
	actionSyncYaml, _ := actionYamlFS.ReadFile("action/sync.yaml")
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

		sync := caction.Sync{
			Keyring: p.k,
			Streams: streams,

			DomainDir:   ".",
			BuildDir:    ".plasma/package/compose/merged",
			PackagesDir: ".plasma/package/compose/packages",

			DryRun:                dryRun,
			FilterByResourceUsage: filterByResourceUsage,
			TimeDepth:             timeDepth,
			AllowOverride:         allowOverride,
			VaultPass:             vaultpass,
			ShowProgress:          !hideProgress,
		}

		sync.SetLogger(log)
		sync.SetTerm(term)
		return sync.Execute()
	}))

	// component:depend action
	actionDependYaml, _ := actionYamlFS.ReadFile("action/depend.yaml")
	da := action.NewFromYAML("component:depend", actionDependYaml)
	da.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)

		input := a.Input()
		source := input.Opt("source").(string)
		operations := action.InputArgSlice[string](input, "operations")

		// Only validate source for show mode (no operations)
		if len(operations) == 0 {
			if _, err := os.Stat(source); os.IsNotExist(err) {
				term.Warning().Printfln("%s doesn't exist, fallback to current dir", source)
				source = "."
			} else {
				term.Info().Printfln("Selected source is %s", source)
			}
		}

		showPaths := input.Opt("path").(bool)
		showTree := input.Opt("tree").(bool)
		depth := int8(input.Opt("depth").(int)) //nolint:gosec
		if depth == 0 {
			return fmt.Errorf("depth value should not be zero")
		}

		target := input.Arg("target").(string)
		depend := &caction.Depend{
			Target:     target,
			Operations: operations,
			Source:     source,
			Path:       showPaths,
			Tree:       showTree,
			Depth:      depth,
		}
		depend.SetLogger(log)
		depend.SetTerm(term)
		return depend.Execute()
	}))

	// component:configure action (unified)
	actionConfigureYaml, _ := actionYamlFS.ReadFile("action/configure.yaml")
	ca := action.NewFromYAML("component:configure", actionConfigureYaml)
	ca.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)
		input := a.Input()

		// Get arguments (may be nil)
		key := ""
		if input.Arg("key") != nil {
			key = input.Arg("key").(string)
		}
		value := ""
		if input.Arg("value") != nil {
			value = input.Arg("value").(string)
		}

		configure := &caction.Configure{
			Key:   key,
			Value: value,

			Get:      input.Opt("get").(bool),
			List:     input.Opt("list").(bool),
			Validate: input.Opt("validate").(bool),
			Generate: input.Opt("generate").(bool),

			At: input.Opt("at").(string),

			Vault:      input.Opt("vault").(bool),
			Format:     input.Opt("format").(string),
			Strict:     input.Opt("strict").(bool),
			YesIAmSure: input.Opt("yes-i-am-sure").(bool),
		}
		configure.SetLogger(log)
		configure.SetTerm(term)
		return configure.Execute()
	}))

	// component:attach action
	actionAttachYaml, _ := actionYamlFS.ReadFile("action/attach.yaml")
	aa := action.NewFromYAML("component:attach", actionAttachYaml)
	aa.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)
		input := a.Input()

		attach := &caction.Attach{
			Component: input.Arg("component").(string),
			Chassis:   input.Arg("chassis").(string),
			Source:    input.Opt("source").(string),
		}
		attach.SetLogger(log)
		attach.SetTerm(term)
		return attach.Execute()
	}))

	// component:detach action
	actionDetachYaml, _ := actionYamlFS.ReadFile("action/detach.yaml")
	dta := action.NewFromYAML("component:detach", actionDetachYaml)
	dta.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)
		input := a.Input()

		detach := &caction.Detach{
			Component: input.Arg("component").(string),
			Chassis:   input.Arg("chassis").(string),
			Source:    input.Opt("source").(string),
		}
		detach.SetLogger(log)
		detach.SetTerm(term)
		return detach.Execute()
	}))

	return []*action.Action{ba, sa, da, ca, aa, dta}, nil
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

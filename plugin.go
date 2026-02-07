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
	"github.com/plasmash/plasmactl-model/pkg/model"

	"github.com/plasmash/plasmactl-component/actions/attach"
	"github.com/plasmash/plasmactl-component/actions/bump"
	"github.com/plasmash/plasmactl-component/actions/configure"
	"github.com/plasmash/plasmactl-component/actions/depend"
	"github.com/plasmash/plasmactl-component/actions/detach"
	"github.com/plasmash/plasmactl-component/actions/list"
	"github.com/plasmash/plasmactl-component/actions/query"
	"github.com/plasmash/plasmactl-component/actions/show"
	"github.com/plasmash/plasmactl-component/actions/sync"
)

//go:embed actions/*/*.yaml
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
	actionBumpYaml, _ := actionYamlFS.ReadFile("actions/bump/bump.yaml")
	ba := action.NewFromYAML("component:bump", actionBumpYaml)
	ba.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		dryRun := input.Opt("dry-run").(bool)
		last := input.Opt("last").(bool)

		log, _, _, term := getLogger(a)

		bump := bump.Bump{Last: last, DryRun: dryRun}
		bump.SetLogger(log)
		bump.SetTerm(term)
		return bump.Execute()
	}))

	// component:sync action
	actionSyncYaml, _ := actionYamlFS.ReadFile("actions/sync/sync.yaml")
	sa := action.NewFromYAML("component:sync", actionSyncYaml)
	sa.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		dryRun := input.Opt("dry-run").(bool)
		allowOverride := input.Opt("allow-override").(bool)
		filterByComponentUsage := input.Opt("chassis").(bool)
		timeDepth := input.Opt("time-depth").(string)
		vaultpass := input.Opt("vault-pass").(string)

		log, logLevel, streams, term := getLogger(a)
		hideProgress := input.Opt("hide-progress").(bool)
		if logLevel > 0 {
			hideProgress = true
		}

		sync := sync.Sync{
			Keyring: p.k,
			Streams: streams,

			DomainDir:   ".",
			BuildDir:    model.MergedSrcDir,
			PackagesDir: model.PackagesDir,

			DryRun:                dryRun,
			FilterByComponentUsage: filterByComponentUsage,
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
	actionDependYaml, _ := actionYamlFS.ReadFile("actions/depend/depend.yaml")
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
				log.Debug("selected source", "path", source)
			}
		}

		showPaths := input.Opt("path").(bool)
		showTree := input.Opt("tree").(bool)
		showReverse := input.Opt("reverse").(bool)
		showBuild := input.Opt("build").(bool)
		depth := int8(input.Opt("depth").(int)) //nolint:gosec
		if depth == 0 {
			return fmt.Errorf("depth value should not be zero")
		}

		target := input.Arg("target").(string)
		depend := &depend.Depend{
			Target:     target,
			Operations: operations,
			Source:     source,
			Path:       showPaths,
			Tree:       showTree,
			Reverse:    showReverse,
			Depth:      depth,
			Build:      showBuild,
		}
		depend.SetLogger(log)
		depend.SetTerm(term)
		return depend.Execute()
	}))

	// component:configure action (unified)
	actionConfigureYaml, _ := actionYamlFS.ReadFile("actions/configure/configure.yaml")
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

		configure := &configure.Configure{
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
	actionAttachYaml, _ := actionYamlFS.ReadFile("actions/attach/attach.yaml")
	aa := action.NewFromYAML("component:attach", actionAttachYaml)
	aa.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)
		input := a.Input()

		attach := &attach.Attach{
			Component: input.Arg("component").(string),
			Chassis:   input.Arg("chassis").(string),
			Source:    input.Opt("source").(string),
		}
		attach.SetLogger(log)
		attach.SetTerm(term)
		return attach.Execute()
	}))

	// component:detach action
	actionDetachYaml, _ := actionYamlFS.ReadFile("actions/detach/detach.yaml")
	dta := action.NewFromYAML("component:detach", actionDetachYaml)
	dta.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		log, _, _, term := getLogger(a)
		input := a.Input()

		detach := &detach.Detach{
			Component: input.Arg("component").(string),
			Chassis:   input.Arg("chassis").(string),
			Source:    input.Opt("source").(string),
		}
		detach.SetLogger(log)
		detach.SetTerm(term)
		return detach.Execute()
	}))

	// component:query action
	actionQueryYaml, _ := actionYamlFS.ReadFile("actions/query/query.yaml")
	qa := action.NewFromYAML("component:query", actionQueryYaml)
	qa.SetRuntime(action.NewFnRuntimeWithResult(func(_ context.Context, a *action.Action) (any, error) {
		log, _, _, term := getLogger(a)
		input := a.Input()

		q := &query.Query{
			Identifier: input.Arg("identifier").(string),
			Kind:       input.Opt("kind").(string),
		}
		q.SetLogger(log)
		q.SetTerm(term)
		err := q.Execute()
		return q.Result(), err
	}))

	// component:list action
	actionListYaml, _ := actionYamlFS.ReadFile("actions/list/list.yaml")
	la := action.NewFromYAML("component:list", actionListYaml)
	la.SetRuntime(action.NewFnRuntimeWithResult(func(_ context.Context, a *action.Action) (any, error) {
		log, _, _, term := getLogger(a)
		input := a.Input()

		l := &list.List{
			Tree:    input.Opt("tree").(bool),
			Kind:    input.Opt("kind").(string),
			All:     input.Opt("all").(bool),
			Orphans: input.Opt("orphans").(bool),
		}
		l.SetLogger(log)
		l.SetTerm(term)
		err := l.Execute()
		return l.Result(), err
	}))

	// component:show action
	actionShowYaml, _ := actionYamlFS.ReadFile("actions/show/show.yaml")
	sha := action.NewFromYAML("component:show", actionShowYaml)
	sha.SetRuntime(action.NewFnRuntimeWithResult(func(_ context.Context, a *action.Action) (any, error) {
		log, _, _, term := getLogger(a)
		input := a.Input()

		comp := ""
		if v := input.Arg("component"); v != nil {
			comp = v.(string)
		}

		sh := &show.Show{
			Component: comp,
		}
		sh.SetLogger(log)
		sh.SetTerm(term)
		err := sh.Execute()
		return sh.Result(), err
	}))

	return []*action.Action{ba, sa, da, ca, aa, dta, qa, la, sha}, nil
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

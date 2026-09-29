package sync

import (
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/launchrctl/launchr"
)

const (
	metaRel     = "meta/plasma.yaml"
	v2Component = "interaction/services/mail_postfix"   // v2: <layer>/<kind>/<name>
	v1Component = "interaction/services/roles/plan_api" // v1: <layer>/<kind>/roles/<name>
)

// Both layouts parse to layer/kind/name; the v1 roles/ infix is skipped.
func TestProcessComponentPath(t *testing.T) {
	tests := []struct {
		path, layer, kind, name string
		wantErr                 bool
	}{
		{v2Component + "/" + metaRel, "interaction", "services", "mail_postfix", false},
		{v1Component + "/" + metaRel, "interaction", "services", "plan_api", false},
		{"interaction/services", "", "", "", true},
	}
	for _, tt := range tests {
		layer, kind, name, err := ProcessComponentPath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tt.path, err, tt.wantErr)
			continue
		}
		if layer != tt.layer || kind != tt.kind || name != tt.name {
			t.Errorf("%s: got %s/%s/%s, want %s/%s/%s", tt.path, layer, kind, name, tt.layer, tt.kind, tt.name)
		}
	}
}

// A component rebuilt from its name lives at <layer>/<kind>/<name> — the shape
// of the merged model.
func TestNewComponent_defaultDir(t *testing.T) {
	c, err := NewComponent("foundation.applications.auth", "/prefix")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.BuildMetaPath(), filepath.FromSlash("foundation/applications/auth/"+metaRel); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A discovered component remembers its real directory, so meta reads honor the
// v1 roles/ infix that the name alone cannot encode.
func TestBuildComponentFromPath_layouts(t *testing.T) {
	root := t.TempDir()
	touch(t, root, v2Component+"/"+metaRel)
	touch(t, root, v1Component+"/"+metaRel)

	tests := []struct{ path, wantName, wantMeta string }{
		{v2Component + "/tasks/configuration.yaml", "interaction.services.mail_postfix", v2Component + "/" + metaRel},
		{v1Component + "/" + metaRel, "interaction.services.plan_api", v1Component + "/" + metaRel},
	}
	for _, tt := range tests {
		c := BuildComponentFromPath(tt.path, root)
		if c == nil {
			t.Fatalf("%s: expected a component", tt.path)
		}
		if c.GetName() != tt.wantName {
			t.Errorf("%s: name %q, want %q", tt.path, c.GetName(), tt.wantName)
		}
		if got := c.BuildMetaPath(); got != filepath.FromSlash(tt.wantMeta) {
			t.Errorf("%s: meta %q, want %q", tt.path, got, tt.wantMeta)
		}
		if !c.IsValidComponent() {
			t.Errorf("%s: meta must be found on disk", tt.path)
		}
	}

	// No meta on disk -> not a component.
	if c := BuildComponentFromPath("interaction/services/ghost/tasks/main.yaml", root); c != nil {
		t.Fatalf("expected nil for a directory without meta, got %q", c.GetName())
	}
}

// One layers root can hold both layouts (a v1 package checkout); the inventory
// must discover components in each.
func TestNewInventory_discoversBothLayouts(t *testing.T) {
	root := t.TempDir()
	touch(t, root, v2Component+"/"+metaRel)
	touch(t, root, v1Component+"/"+metaRel)
	touch(t, root, v2Component+"/templates/main.cf.j2") // not a meta/tasks file: ignored

	inv, err := NewInventory(root, launchr.NewConsoleLogger(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	got := inv.GetComponentsMap().Keys()
	sort.Strings(got)
	want := []string{"interaction.services.mail_postfix", "interaction.services.plan_api"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("components = %v, want %v", got, want)
	}
}

package sync

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// touch creates a small file at root/rel, creating parent directories.
func touch(t *testing.T, root, rel string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", p, err)
	}
	if err := os.WriteFile(p, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// assertFileMap compares layer -> files maps ignoring file order.
func assertFileMap(t *testing.T, got, want map[string][]string) {
	t.Helper()
	for _, m := range []map[string][]string{got, want} {
		for _, files := range m {
			sort.Strings(files)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("files mismatch\n got: %v\nwant: %v", got, want)
	}
}

// v2 layout: <layer>/variables/<group>/{vars.yaml,vault.yaml}. Anything else
// under variables/, and the v1 group_vars/ directory, must be ignored.
func TestFindVarsFiles_v2Layout(t *testing.T) {
	root := t.TempDir()
	group := "interaction/variables/platform.interaction.communication/"
	touch(t, root, group+"vars.yaml")
	touch(t, root, group+"vault.yaml")
	touch(t, root, group+"README.md") // not a vars file
	touch(t, root, "platform/variables/platform/vars.yaml")
	touch(t, root, "interaction/group_vars/legacy/vars.yaml") // v1: ignored

	got, err := NewFilesCrawler(root).FindVarsFiles("")
	if err != nil {
		t.Fatal(err)
	}
	assertFileMap(t, got, map[string][]string{
		"interaction": {group + "vars.yaml", group + "vault.yaml"},
		"platform":    {"platform/variables/platform/vars.yaml"},
	})
}

// v2 layout: <layer>/<kind>/<name>/templates/**/*.j2 and
// <layer>/<kind>/<name>/tasks/configuration.yaml. The v1 roles/ infix shifts
// the kind segment and must not match.
func TestFindComponentsFiles_v2Layout(t *testing.T) {
	root := t.TempDir()
	comp := "interaction/services/mail_postfix/"
	touch(t, root, comp+"templates/main.cf.j2")
	touch(t, root, comp+"templates/sub/extra.conf.j2")
	touch(t, root, comp+"templates/README.md") // not .j2
	touch(t, root, comp+"tasks/configuration.yaml")
	touch(t, root, comp+"tasks/main.yaml")                             // only configuration.yaml counts
	touch(t, root, comp+"meta/plasma.yaml")                            // not a source file
	touch(t, root, "interaction/services/roles/legacy/templates/x.j2") // v1: ignored

	got, err := NewFilesCrawler(root).FindComponentsFiles("")
	if err != nil {
		t.Fatal(err)
	}
	assertFileMap(t, got, map[string][]string{
		"interaction": {
			comp + "templates/main.cf.j2",
			comp + "templates/sub/extra.conf.j2",
			comp + "tasks/configuration.yaml",
		},
	})
}

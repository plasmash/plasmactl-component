package bump

import (
	"os"
	"path/filepath"
	"testing"
)

// writeComponentMeta lays down a v2-layout component (src/<layer>/<kind>/<role>/meta/plasma.yaml)
// under root and returns nothing. The version goes into plasma.version.
func writeComponentMeta(t *testing.T, root, componentRel, version string) {
	t.Helper()
	metaDir := filepath.Join(root, componentRel, "meta")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", metaDir, err)
	}
	content := "plasma:\n  version: " + version + "\n"
	if err := os.WriteFile(filepath.Join(metaDir, "plasma.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

// A commit touching a component's meta/plasma.yaml in the v2 src/ layout must
// resolve to that component. Regression: the leading "src/" segment used to
// shift the layer/kind/role parse by one (src/foundation/applications instead of
// foundation/applications/cluster), yielding an invalid component bump silently
// dropped -> "No component to update".
func TestComponentFromPath_resolvesV2SrcComponent(t *testing.T) {
	root := t.TempDir()
	writeComponentMeta(t, root, "src/foundation/applications/cluster", "37ba1b30aae2e")
	t.Chdir(root)

	c := componentFromPath("src/foundation/applications/cluster/meta/plasma.yaml")
	if c == nil {
		t.Fatal("expected component for src/-prefixed path, got nil")
	}
	if got, want := c.GetName(), "foundation.applications.cluster"; got != want {
		t.Fatalf("component name = %q, want %q", got, want)
	}
	if !c.IsValidComponent() {
		t.Fatal("component should be valid: meta/plasma.yaml exists under src/")
	}
}

// A change confined to a component's actions/ directory must not trigger a bump.
func TestComponentFromPath_skipsActionsDir(t *testing.T) {
	root := t.TempDir()
	writeComponentMeta(t, root, "src/foundation/applications/cluster", "37ba1b30aae2e")
	t.Chdir(root)

	c := componentFromPath("src/foundation/applications/cluster/actions/deploy/action.yaml")
	if c != nil {
		t.Fatalf("expected nil for an actions/ change, got component %q", c.GetName())
	}
}

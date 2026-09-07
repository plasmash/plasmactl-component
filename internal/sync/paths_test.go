package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LayersRoot points at <repo>/src for a v2 checkout and at the checkout itself
// for a v1 one; the prefix maps layer-relative paths back for git lookups.
func TestLayersRoot(t *testing.T) {
	v2 := t.TempDir()
	if err := os.Mkdir(filepath.Join(v2, SrcDir), 0o750); err != nil {
		t.Fatal(err)
	}
	v1 := t.TempDir()

	tests := []struct{ name, repo, wantRoot, wantPrefix string }{
		{"v2 checkout with src/", v2, filepath.Join(v2, SrcDir), SrcDir},
		{"v1 checkout without src/", v1, v1, ""},
	}
	for _, tt := range tests {
		root, prefix := LayersRoot(tt.repo)
		if root != tt.wantRoot || prefix != tt.wantPrefix {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tt.name, root, prefix, tt.wantRoot, tt.wantPrefix)
		}
	}
}

// SourceRoot is the strict variant used for the domain: src/ is required.
func TestSourceRoot_requiresSrc(t *testing.T) {
	repo := t.TempDir()
	if _, err := SourceRoot(repo); err == nil {
		t.Fatal("expected an error without src/")
	}
	if err := os.Mkdir(filepath.Join(repo, SrcDir), 0o750); err != nil {
		t.Fatal(err)
	}
	got, err := SourceRoot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(repo, SrcDir); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The component directory is 3 segments in the v2 layout and 4 in v1 (roles/).
func TestComponentDirParts(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"interaction/services/mail_postfix/meta/plasma.yaml", 3},
		{"interaction/services/roles/plan_api/meta/plasma.yaml", 4},
		{"interaction/services", 3}, // too short to tell: v2 by default
	}
	for _, tt := range tests {
		if got := ComponentDirParts(strings.Split(tt.path, "/")); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.path, got, tt.want)
		}
	}
}

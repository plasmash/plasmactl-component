package sync

import (
	"fmt"
	"os"
	"path/filepath"
)

// SrcDir is the directory that holds the layers of a v2 repository:
// <repo>/src/<layer>/<kind>/<name>. model:compose normalizes every package
// (v1 included) into this shape when building the merged model, so it is the
// only layout sync has to understand.
const SrcDir = "src"

// SourceRoot returns the layers root of a v2 repository (<repoDir>/src).
// Paths relative to it start with the layer name — the same shape as the
// merged model — which keeps the crawler, the merged lookups and the git
// lookups consistent. A missing src/ is an error, not a silent empty result.
func SourceRoot(repoDir string) (string, error) {
	root := filepath.Join(repoDir, SrcDir)
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("v2 layout expected: directory %s not found", root)
	}
	return root, nil
}

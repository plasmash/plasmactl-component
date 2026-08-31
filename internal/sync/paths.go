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

// RolesDir is the infix directory of the v1 layout: <layer>/<kind>/roles/<name>.
const RolesDir = "roles"

// LayersRoot returns the layers root of a repository checkout and the prefix
// that maps layer-relative paths back to repository-relative ones:
// (<repo>/src, "src") for the v2 layout, (<repo>, "") for a v1 checkout.
// Unlike SourceRoot it tolerates a missing src/: raw v1 packages keep their
// original shape — model:compose normalizes only the merged copy.
func LayersRoot(repoDir string) (root, gitPrefix string) {
	root = filepath.Join(repoDir, SrcDir)
	if st, err := os.Stat(root); err == nil && st.IsDir() {
		return root, SrcDir
	}
	return repoDir, ""
}

// ComponentDirParts returns how many leading segments of a layer-relative path
// form the component directory: 3 for <layer>/<kind>/<name> (v2),
// 4 for <layer>/<kind>/roles/<name> (v1).
func ComponentDirParts(parts []string) int {
	if len(parts) > 2 && parts[2] == RolesDir {
		return 4
	}
	return 3
}

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

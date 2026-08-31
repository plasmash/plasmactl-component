package sync

import (
	"os"
	"path/filepath"
	"strings"
)

// The crawler understands the v2 layout only, relative to a layers root
// (<domain>/src or the merged model):
//
//	<layer>/variables/<group>/{vars.yaml,vault.yaml}
//	<layer>/<kind>/<name>/templates/**/*.j2
//	<layer>/<kind>/<name>/tasks/configuration.yaml
//
// v1 shapes (group_vars/, roles/) never reach sync: model:compose strips them
// while building the merged model, and the domain is v2 by definition.
const (
	variablesDir = "variables"
	templatesDir = "templates"
	tasksDir     = "tasks"
)

// FilesCrawler is a type that represents a crawler for components in a given directory.
type FilesCrawler struct {
	rootDir string
}

// NewFilesCrawler creates a new instance of FilesCrawler with initialized taskSources and templateSources maps.
func NewFilesCrawler(directory string) *FilesCrawler {
	return &FilesCrawler{
		rootDir: directory,
	}
}

// FindVarsFiles returns variables files grouped by layer:
// <layer>/variables/<group>/{vars.yaml,vault.yaml}.
// If platform (layer) is empty, search across all layers.
func (cr *FilesCrawler) FindVarsFiles(platform string) (map[string][]string, error) {
	// parts: [layer, variables, group, file]
	const (
		partsCount   = 3
		platformPart = 0
		kindPart     = 1
	)

	files := make(map[string][]string)
	dir := filepath.Join(cr.rootDir, platform)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath := strings.TrimPrefix(path, cr.rootDir+"/")
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if strings.Contains(path, "scripts") {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		parts := strings.Split(relPath, "/")
		if len(parts) > partsCount && (platform == "" || parts[platformPart] == platform) {
			if parts[kindPart] == variablesDir {
				filename := filepath.Base(path)
				if filename == "vars.yaml" || filename == vaultFile {
					files[parts[platformPart]] = append(files[parts[platformPart]], relPath)
				}
			}
		}

		return nil
	})

	return files, err
}

// FindComponentsFiles returns component source files grouped by layer:
// <layer>/<kind>/<name>/templates/**/*.j2 and
// <layer>/<kind>/<name>/tasks/configuration.yaml.
// If platform (layer) is empty, search across all layers.
func (cr *FilesCrawler) FindComponentsFiles(platform string) (map[string][]string, error) {
	// parts: [layer, kind, name, templates|tasks, file...]
	const (
		partsCount   = 3
		platformPart = 0
		kindPart     = 3
	)

	files := make(map[string][]string)
	dir := filepath.Join(cr.rootDir, platform)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath := strings.TrimPrefix(path, cr.rootDir+"/")
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if strings.Contains(path, "scripts") {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		parts := strings.Split(relPath, "/")
		if len(parts) > partsCount && (platform == "" || parts[platformPart] == platform) {
			if parts[kindPart] == templatesDir {
				ext := filepath.Ext(path)
				if ext != ".j2" {
					return nil
				}
				files[parts[platformPart]] = append(files[parts[platformPart]], relPath)

			} else if parts[kindPart] == tasksDir && filepath.Base(path) == "configuration.yaml" {
				files[parts[platformPart]] = append(files[parts[platformPart]], relPath)
			}
		}

		return nil
	})

	return files, err
}

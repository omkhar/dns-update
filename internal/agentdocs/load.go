package agentdocs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

var readSourceFile = os.ReadFile

// Load reads the canonical source tree under docs/agents.
func Load(root string) ([]Source, error) {
	if err := rejectSymlinkRoot(root); err != nil {
		return nil, err
	}

	contractPath := filepath.Join(root, "docs", "agents", "contract.md")
	skillPattern := filepath.Join(root, "docs", "agents", "skills", "*.md")

	skillPaths, _ := filepath.Glob(skillPattern)
	sort.Strings(skillPaths)

	paths := append([]string{contractPath}, skillPaths...)
	sources := make([]Source, 0, len(paths))
	for _, absPath := range paths {
		info, err := lstatPath(absPath)
		if err != nil {
			return nil, fmt.Errorf("lstat %s: %w", absPath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("canonical source must be a regular file: %s", absPath)
		}
		data, err := readSourceFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", absPath, err)
		}
		relPath, _ := filepath.Rel(root, absPath)
		relPath = filepath.ToSlash(relPath)
		source, err := ParseDocument(relPath, data)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, nil
}

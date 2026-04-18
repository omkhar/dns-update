package agentdocs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
		data, err := os.ReadFile(absPath)
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

package agentdocs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrOutOfDate reports that generated outputs differ from what is on disk.
var ErrOutOfDate = errors.New("generated agent docs out of date")

// Plan loads the source tree and renders all provider projections.
func Plan(root string) ([]Output, error) {
	sources, err := Load(root)
	if err != nil {
		return nil, err
	}

	contract, skills, err := splitSources(sources)
	if err != nil {
		return nil, err
	}

	outputs := make([]Output, 0, len(skills)*3+3)
	seen := map[string]struct{}{}
	for _, output := range RenderContract(contract, skills) {
		seen[output.Path] = struct{}{}
		outputs = append(outputs, output)
	}
	for _, skill := range skills {
		for _, output := range RenderSkill(skill) {
			if _, ok := seen[output.Path]; ok {
				return nil, fmt.Errorf("duplicate generated path %s", output.Path)
			}
			seen[output.Path] = struct{}{}
			outputs = append(outputs, output)
		}
	}

	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].Path < outputs[j].Path
	})
	return outputs, nil
}

// Write updates all generated projections on disk.
func Write(root string) error {
	outputs, err := Plan(root)
	if err != nil {
		return err
	}

	stalePaths, err := staleManagedPaths(root, outputs)
	if err != nil {
		return err
	}
	for _, stalePath := range stalePaths {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(stalePath))); err != nil {
			return fmt.Errorf("remove stale %s: %w", stalePath, err)
		}
	}
	for _, output := range outputs {
		absPath := filepath.Join(root, filepath.FromSlash(output.Path))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, []byte(output.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", absPath, err)
		}
	}
	return nil
}

// Check compares the generated projections with what is already on disk.
func Check(root string) ([]Mismatch, error) {
	outputs, err := Plan(root)
	if err != nil {
		return nil, err
	}

	var mismatches []Mismatch
	stalePaths, err := staleManagedPaths(root, outputs)
	if err != nil {
		return nil, err
	}
	for _, stalePath := range stalePaths {
		mismatches = append(mismatches, Mismatch{
			Path:  stalePath,
			Stale: true,
		})
	}

	for _, output := range outputs {
		absPath := filepath.Join(root, filepath.FromSlash(output.Path))
		observed, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				mismatches = append(mismatches, Mismatch{
					Path:     output.Path,
					Expected: output.Content,
					Missing:  true,
				})
				continue
			}
			return nil, fmt.Errorf("read %s: %w", absPath, err)
		}
		if normalizeNewlines(string(observed)) != normalizeNewlines(output.Content) {
			mismatches = append(mismatches, Mismatch{
				Path:     output.Path,
				Expected: output.Content,
				Observed: string(observed),
			})
		}
	}

	if len(mismatches) > 0 {
		return mismatches, ErrOutOfDate
	}
	return nil, nil
}

// FormatMismatch renders one check-mode mismatch for humans and CI logs.
func FormatMismatch(m Mismatch) string {
	if m.Stale {
		return fmt.Sprintf("%s is stale and should be removed", m.Path)
	}
	if m.Missing {
		return fmt.Sprintf("%s is missing", m.Path)
	}
	return fmt.Sprintf("%s is out of date", m.Path)
}

// Summary returns a short, stable description of all mismatches.
func Summary(mismatches []Mismatch) string {
	if len(mismatches) == 0 {
		return ""
	}
	lines := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		lines = append(lines, FormatMismatch(mismatch))
	}
	return strings.Join(lines, "\n")
}

func staleManagedPaths(root string, outputs []Output) ([]string, error) {
	expected := make(map[string]struct{}, len(outputs))
	for _, output := range outputs {
		expected[output.Path] = struct{}{}
	}

	current, err := managedPaths(root)
	if err != nil {
		return nil, err
	}

	stale := make([]string, 0)
	for _, currentPath := range current {
		if _, ok := expected[currentPath]; ok {
			continue
		}
		stale = append(stale, currentPath)
	}
	sort.Strings(stale)
	return stale, nil
}

func managedPaths(root string) ([]string, error) {
	paths := make([]string, 0)
	paths = appendIfRegular(paths, root, "AGENTS.md")
	paths = appendIfRegular(paths, root, "CLAUDE.md")
	paths = appendIfRegular(paths, root, "GEMINI.md")

	for _, relRoot := range []string{
		filepath.Join(".agents", "skills"),
		filepath.Join(".claude", "skills"),
		filepath.Join(".gemini", "commands"),
	} {
		absRoot := filepath.Join(root, relRoot)
		if _, err := os.Stat(absRoot); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}

		if err := filepath.WalkDir(absRoot, func(absPath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !entry.Type().IsRegular() {
				return nil
			}
			relPath, _ := filepath.Rel(root, absPath)
			paths = append(paths, filepath.ToSlash(relPath))
			return nil
		}); err != nil {
			return nil, err
		}
	}

	sort.Strings(paths)
	return paths, nil
}

func appendIfRegular(paths []string, root, relPath string) []string {
	absPath := filepath.Join(root, relPath)
	info, err := os.Stat(absPath)
	if err != nil || !info.Mode().IsRegular() {
		return paths
	}
	return append(paths, relPath)
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func splitSources(sources []Source) (Source, []Source, error) {
	var contract Source
	skills := make([]Source, 0, len(sources))
	for _, source := range sources {
		switch source.Kind {
		case KindContract:
			if contract.Path != "" {
				return Source{}, nil, fmt.Errorf("multiple contract documents found")
			}
			contract = source
		case KindSkill:
			skills = append(skills, source)
		default:
			return Source{}, nil, fmt.Errorf("unsupported kind %q", source.Kind)
		}
	}
	if contract.Path == "" {
		return Source{}, nil, fmt.Errorf("missing contract document")
	}
	return contract, skills, nil
}

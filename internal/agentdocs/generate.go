package agentdocs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	lstatPath  = os.Lstat
	removePath = os.Remove
	makeDirs   = os.MkdirAll
	walkDir    = filepath.WalkDir
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
		if err := removePath(filepath.Join(root, filepath.FromSlash(stalePath))); err != nil {
			return fmt.Errorf("remove stale %s: %w", stalePath, err)
		}
	}
	for _, output := range outputs {
		absPath := filepath.Join(root, filepath.FromSlash(output.Path))
		if err := ensureSafeWritePath(root, output.Path); err != nil {
			return err
		}
		if err := makeDirs(filepath.Dir(absPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(absPath, []byte(output.Content), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", absPath, err)
		}
	}
	return nil
}

func ensureSafeWritePath(root, relPath string) error {
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	relToRoot, _ := filepath.Rel(root, absPath)
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("generated path escapes repository root: %s", relPath)
	}

	symlinkPath, err := firstManagedSymlink(root, relPath)
	if err != nil {
		return err
	}
	if symlinkPath != "" {
		return fmt.Errorf("managed output path %s traverses symlink %s", relPath, symlinkPath)
	}
	return nil
}

func rejectSymlinkRoot(root string) error {
	info, err := lstatPath(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository root must not be a symlink: %s", root)
	}
	return nil
}

func firstManagedSymlink(root, relPath string) (string, error) {
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	relToRoot, _ := filepath.Rel(root, absPath)
	current := root
	parts := strings.Split(relToRoot, string(os.PathSeparator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := lstatPath(current)
		if os.IsNotExist(err) {
			if i == len(parts)-1 {
				return "", nil
			}
			continue
		}
		if err != nil {
			return "", fmt.Errorf("lstat %s: %w", relPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return filepath.ToSlash(filepath.Join(parts[:i+1]...)), nil
		}
	}
	return "", nil
}

func readManagedOutput(absPath string) (observed string, missing bool, invalid bool, err error) {
	info, err := lstatPath(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", true, false, nil
		}
		return "", false, false, fmt.Errorf("read %s: %w", absPath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", false, true, nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", false, false, fmt.Errorf("read %s: %w", absPath, err)
	}
	return string(data), false, false, nil
}

func statManagedRoot(absRoot, relRoot string) (bool, error) {
	info, err := lstatPath(absRoot)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s: not a directory", relRoot)
	}
	return true, nil
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
		observed, missing, invalid, err := readManagedOutput(absPath)
		if err != nil {
			return nil, err
		}
		if missing {
			mismatches = append(mismatches, Mismatch{
				Path:     output.Path,
				Expected: output.Content,
				Missing:  true,
			})
			continue
		}
		if invalid {
			mismatches = append(mismatches, Mismatch{
				Path:    output.Path,
				Invalid: true,
			})
			continue
		}
		if normalizeNewlines(observed) != normalizeNewlines(output.Content) {
			mismatches = append(mismatches, Mismatch{
				Path:     output.Path,
				Expected: output.Content,
				Observed: observed,
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
	if m.Invalid {
		return fmt.Sprintf("%s is invalid and should be replaced", m.Path)
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
	paths = appendIfFile(paths, root, "AGENTS.md")
	paths = appendIfFile(paths, root, "CLAUDE.md")
	paths = appendIfFile(paths, root, "GEMINI.md")

	for _, relRoot := range []string{
		filepath.Join(".agents", "skills"),
		filepath.Join(".claude", "skills"),
		filepath.Join(".gemini", "commands"),
	} {
		symlinkPath, err := firstManagedSymlink(root, filepath.ToSlash(relRoot))
		if err != nil {
			return nil, err
		}
		if symlinkPath != "" {
			paths = append(paths, symlinkPath)
			continue
		}

		absRoot := filepath.Join(root, relRoot)
		exists, err := statManagedRoot(absRoot, filepath.ToSlash(relRoot))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}

		if err := walkDir(absRoot, func(absPath string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
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

func appendIfFile(paths []string, root, relPath string) []string {
	absPath := filepath.Join(root, relPath)
	info, err := lstatPath(absPath)
	if err != nil || info.IsDir() {
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

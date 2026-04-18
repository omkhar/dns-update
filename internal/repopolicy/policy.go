package repopolicy

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Finding struct {
	Path    string
	Message string
}

type pathRule struct {
	message string
	match   func(path string) bool
}

type contentRule struct {
	message string
	needle  string
}

var blockedPathRules = []pathRule{
	{
		message: "tracked analysis artifacts do not belong in the public repository",
		match: func(path string) bool {
			return path == "security_best_practices_report.md"
		},
	},
	{
		message: "tracked build or test detritus does not belong in the public repository",
		match: func(path string) bool {
			return strings.HasPrefix(path, "out/") ||
				strings.HasPrefix(path, "coverage/") ||
				strings.HasPrefix(path, "reports/") ||
				strings.HasPrefix(path, ".stryker-tmp/")
		},
	},
	{
		message: "tracked transient compiler state does not belong in the public repository",
		match: func(path string) bool {
			return strings.HasSuffix(path, ".tsbuildinfo")
		},
	},
}

var blockedContentRules = []contentRule{
	{
		message: "absolute macOS home paths do not belong in public repository content",
		needle:  joinFragments("/Us", "ers/"),
	},
	{
		message: "absolute Windows home paths do not belong in public repository content",
		needle:  joinFragments(`:\`, "Us", `ers\`),
	},
	{
		message: "private local usernames do not belong in public repository content",
		needle:  joinFragments("omkhar", "anara", "saratnam"),
	},
	{
		message: "non-public repository references do not belong in public repository content",
		needle:  joinFragments("cloudflare-", "site-platform-", "ts"),
	},
	{
		message: "non-public repository references do not belong in public repository content",
		needle:  joinFragments("work", "cell"),
	},
	{
		message: "non-public visibility markers do not belong in public repository content",
		needle:  joinFragments("private", "-only"),
	},
	{
		message: "non-public visibility markers do not belong in public repository content",
		needle:  joinFragments("internal", "-only"),
	},
}

func Check(root string) ([]Finding, error) {
	files, err := projectFiles(root)
	if err != nil {
		return nil, err
	}

	findings := make([]Finding, 0)
	for _, path := range files {
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		data, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}

		for _, rule := range blockedPathRules {
			if rule.match(path) {
				findings = append(findings, Finding{
					Path:    path,
					Message: rule.message,
				})
			}
		}
		if isBinary(data) {
			continue
		}

		content := normalizeNewlines(string(data))
		for _, rule := range blockedContentRules {
			if strings.Contains(content, rule.needle) {
				findings = append(findings, Finding{
					Path:    path,
					Message: rule.message,
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path == findings[j].Path {
			return findings[i].Message < findings[j].Message
		}
		return findings[i].Path < findings[j].Path
	})

	return findings, nil
}

func projectFiles(root string) ([]string, error) {
	files, err := trackedFiles(root)
	if err == nil {
		return files, nil
	}

	var execErr *exec.ExitError
	if !errors.As(err, &execErr) {
		return nil, err
	}

	files = make([]string, 0)
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, _ := filepath.Rel(root, path)
		if relPath == "." {
			return nil
		}

		relPath = filepath.ToSlash(relPath)
		if entry.IsDir() {
			if relPath == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if !entry.Type().IsRegular() {
			return nil
		}
		files = append(files, relPath)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Strings(files)
	return files, nil
}

func trackedFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	rawFiles := bytes.Split(output, []byte{0})
	files := make([]string, 0, len(rawFiles))
	for _, raw := range rawFiles {
		if len(raw) == 0 {
			continue
		}
		files = append(files, filepath.ToSlash(string(raw)))
	}
	sort.Strings(files)
	return files, nil
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func joinFragments(parts ...string) string {
	return strings.Join(parts, "")
}

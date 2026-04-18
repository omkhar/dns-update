package quality_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"dns-update/internal/agentdocs"
	"dns-update/internal/repopolicy"
)

const (
	skipCoverageEnv = "DNS_UPDATE_SKIP_COVERAGE_TEST"
	skipMutationEnv = "DNS_UPDATE_SKIP_MUTATION_TEST"
)

type mutant struct {
	name string
	file string
	old  string
	new  string
}

func TestAgentArtifactsUpToDate(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	mismatches, err := agentdocs.Check(root)
	if err != nil && !errors.Is(err, agentdocs.ErrOutOfDate) {
		t.Fatalf("agentdocs.Check() error = %v", err)
	}
	if len(mismatches) == 0 {
		return
	}

	t.Fatalf("generated agent artifacts are out of date:\n%s", agentdocs.Summary(mismatches))
}

func TestPublicRepoHygiene(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	findings, err := repopolicy.Check(root)
	if err != nil {
		t.Fatalf("repopolicy.Check() error = %v", err)
	}
	if len(findings) == 0 {
		return
	}

	lines := make([]string, 0, len(findings))
	for _, finding := range findings {
		lines = append(lines, fmt.Sprintf("%s: %s", finding.Path, finding.Message))
	}
	t.Fatalf("public repository hygiene failures:\n%s", strings.Join(lines, "\n"))
}

func TestAgentdocsIntegration(t *testing.T) {
	t.Parallel()
	clone := func(t *testing.T) string { t.Helper(); root := t.TempDir(); if err := copyTree(filepath.Join(repoRoot(t), "docs", "agents"), filepath.Join(root, "docs", "agents")); err != nil { t.Fatalf("copyTree(docs/agents) error = %v", err) }; return root }
	write := func(t *testing.T, path, content string) { t.Helper(); if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatalf("MkdirAll(%s) = %v", path, err) }; if err := os.WriteFile(path, []byte(content), 0o644); err != nil { t.Fatalf("WriteFile(%s) = %v", path, err) } }

	root := clone(t)
	if _, err := agentdocs.Check(t.TempDir()); err == nil || !strings.Contains(err.Error(), "contract.md") { t.Fatalf("agentdocs.Check(missing contract) error = %v, want contract path failure", err) }
	if _, err := agentdocs.Check(root); !errors.Is(err, agentdocs.ErrOutOfDate) { t.Fatalf("agentdocs.Check(missing outputs) error = %v, want %v", err, agentdocs.ErrOutOfDate) }
	stalePath := filepath.Join(root, ".gemini", "commands", "stale.toml")
	write(t, stalePath, "stale\n")
	if mismatches, err := agentdocs.Check(root); !errors.Is(err, agentdocs.ErrOutOfDate) || !strings.Contains(agentdocs.Summary(mismatches), ".gemini/commands/stale.toml is stale and should be removed") { t.Fatalf("agentdocs.Check(stale) = (%v, %v), want stale mismatch", mismatches, err) }
	if err := agentdocs.Write(root); err != nil { t.Fatalf("agentdocs.Write() error = %v", err) }
	write(t, filepath.Join(root, "AGENTS.md"), "drifted\n")
	if mismatches, err := agentdocs.Check(root); !errors.Is(err, agentdocs.ErrOutOfDate) || !strings.Contains(agentdocs.Summary(mismatches), "AGENTS.md is out of date") { t.Fatalf("agentdocs.Check(drifted) = (%v, %v), want AGENTS.md drift", mismatches, err) }
	root = clone(t)
	write(t, filepath.Join(root, ".agents", "skills"), "file\n")
	if _, err := agentdocs.Check(root); err == nil || !strings.Contains(err.Error(), ".agents/skills") { t.Fatalf("agentdocs.Check(managed root file) error = %v, want .agents/skills failure", err) }
	if err := agentdocs.Write(root); err == nil || !strings.Contains(err.Error(), ".agents/skills") { t.Fatalf("agentdocs.Write(managed root file) error = %v, want .agents/skills failure", err) }
}

func TestCoverageThreshold(t *testing.T) {
	t.Parallel()

	if os.Getenv(skipCoverageEnv) == "1" {
		t.Skipf("%s=1", skipCoverageEnv)
	}

	root := repoRoot(t)
	profilePath := filepath.Join(t.TempDir(), "coverage.out")

	if output, err := runCommand(root,
		[]string{
			skipCoverageEnv + "=1",
			skipMutationEnv + "=1",
		},
		"go", "test", "-count=1", "-coverpkg=./...", "-coverprofile="+profilePath, "./...",
	); err != nil {
		t.Fatalf("coverage run failed: %v\n%s", err, output)
	}

	output, err := runCommand(root, nil, "go", "tool", "cover", "-func="+profilePath)
	if err != nil {
		t.Fatalf("go tool cover failed: %v\n%s", err, output)
	}

	total, err := parseCoverageTotal(output)
	if err != nil {
		t.Fatalf("parse coverage total: %v\n%s", err, output)
	}
	if total != 100.0 {
		t.Fatalf("total coverage = %.1f%%, want 100.0%%\n%s", total, output)
	}
}

func TestMutationSuite(t *testing.T) {
	if os.Getenv(skipMutationEnv) == "1" {
		t.Skipf("%s=1", skipMutationEnv)
	}

	root := repoRoot(t)
	mutants := []mutant{
		{
			name: "run_dry_run_guard",
			file: "internal/app/run.go",
			old: "\tif options.DryRun {\n" +
				"\t\tr.logger.Info(\"dry run: planned provider operations\", \"operations\", strings.Join(plan.Summaries(), \"; \"))\n" +
				"\t\treturn nil\n" +
				"\t}\n",
			new: "\tif !options.DryRun {\n" +
				"\t\tr.logger.Info(\"dry run: planned provider operations\", \"operations\", strings.Join(plan.Summaries(), \"; \"))\n" +
				"\t\treturn nil\n" +
				"\t}\n",
		},
		{
			name: "config_ttl_zero_check",
			file: "internal/config/config.go",
			old:  "\tif raw.TTLSeconds == 0 {\n",
			new:  "\tif raw.TTLSeconds != 0 {\n",
		},
		{
			name: "probe_none_sentinel",
			file: "internal/egress/probe.go",
			old:  "\tif rawAddress == \"none\" {\n",
			new:  "\tif rawAddress != \"none\" {\n",
		},
		{
			name: "provider_nil_deletion_path",
			file: "internal/provider/provider.go",
			old: "\tif desired == nil {\n" +
				"\t\toperations := make([]Operation, 0, len(current))\n" +
				"\t\tfor _, record := range current {\n" +
				"\t\t\toperations = append(operations, Operation{\n" +
				"\t\t\t\tKind:    OperationDelete,\n" +
				"\t\t\t\tCurrent: record,\n" +
				"\t\t\t})\n" +
				"\t\t}\n" +
				"\t\treturn operations\n" +
				"\t}\n",
			new: "\tif desired == nil {\n" +
				"\t\treturn nil\n" +
				"\t}\n",
		},
		{
			name: "cloudflare_sdk_retries_disabled",
			file: "internal/provider/cloudflare/client.go",
			old:  "\t\toption.WithMaxRetries(0),\n",
			new:  "\t\toption.WithMaxRetries(2),\n",
		},
	}

	for _, tc := range mutants {
		t.Run(tc.name, func(t *testing.T) {
			workspace := filepath.Join(t.TempDir(), "repo")
			if err := copyTree(root, workspace); err != nil {
				t.Fatalf("copyTree() error = %v", err)
			}
			if err := applyMutant(workspace, tc); err != nil {
				t.Fatalf("applyMutant() error = %v", err)
			}

			output, err := runCommand(workspace,
				[]string{
					skipCoverageEnv + "=1",
					skipMutationEnv + "=1",
				},
				"go", "test", "-count=1", "./...",
			)
			if err == nil {
				t.Fatalf("mutant survived: %s\n%s", tc.name, output)
			}
		})
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func runCommand(dir string, extraEnv []string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func parseCoverageTotal(output string) (float64, error) {
	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, "total:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			break
		}

		raw := strings.TrimSuffix(fields[len(fields)-1], "%")
		total, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, fmt.Errorf("parse total coverage %q: %w", raw, err)
		}
		return total, nil
	}

	return 0, errors.New("total coverage line not found")
}

func copyTree(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relativePath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relativePath == "." {
			return os.MkdirAll(dst, 0o755)
		}

		targetPath := filepath.Join(dst, relativePath)
		switch {
		case info.IsDir():
			return os.MkdirAll(targetPath, info.Mode().Perm())
		case info.Mode().IsRegular():
			return copyFile(path, targetPath, info.Mode().Perm())
		default:
			return fmt.Errorf("unsupported filesystem entry %q", path)
		}
	})
}

func copyFile(src string, dst string, mode os.FileMode) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = source.Close()
	}()

	destination, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(destination, source); err != nil {
		_ = destination.Close()
		return err
	}
	return destination.Close()
}

func applyMutant(root string, tc mutant) error {
	path := filepath.Join(root, filepath.FromSlash(tc.file))
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	content := normalizeNewlines(string(data))
	old := normalizeNewlines(tc.old)
	newValue := normalizeNewlines(tc.new)
	count := strings.Count(content, old)
	if count != 1 {
		return fmt.Errorf("expected one mutation target in %s, found %d", tc.file, count)
	}

	mutated := strings.Replace(content, old, newValue, 1)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mutated), info.Mode().Perm())
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

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
			old:  "\tif dryRun {\n",
			new:  "\tif !dryRun {\n",
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
		tc := tc
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
	for _, line := range strings.Split(output, "\n") {
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
	defer source.Close()

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

	content := string(data)
	count := strings.Count(content, tc.old)
	if count != 1 {
		return fmt.Errorf("expected one mutation target in %s, found %d", tc.file, count)
	}

	mutated := strings.Replace(content, tc.old, tc.new, 1)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(mutated), info.Mode().Perm())
}

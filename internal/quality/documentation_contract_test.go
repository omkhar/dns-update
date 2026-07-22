package quality_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type pinnedRuntime struct {
	Version string `json:"version"`
	SHA     string `json:"sha"`
}

type pinnedContainer struct {
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
}

type runtimeManifest struct {
	Go         string                     `json:"go"`
	Runners    []string                   `json:"runners"`
	Actions    map[string]pinnedRuntime   `json:"actions"`
	Containers map[string]pinnedContainer `json:"containers"`
}

func TestRuntimeManifestMatchesRepository(t *testing.T) {
	root := repoRoot(t)
	manifestData, err := os.ReadFile(filepath.Join(root, "docs", "runtime-versions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest runtimeManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}

	goMod := mustReadContractFile(t, root, "go.mod")
	goVersion := regexp.MustCompile(`(?m)^go (\S+)$`).FindStringSubmatch(goMod)
	if len(goVersion) != 2 || goVersion[1] != manifest.Go {
		t.Fatalf("go.mod version = %v, manifest version = %q", goVersion, manifest.Go)
	}

	workflowRoot := filepath.Join(root, ".github", "workflows")
	actionPattern := regexp.MustCompile(`uses:\s*([^\s@]+)@([0-9a-f]{40})`)
	seenActions := make(map[string]bool)
	err = filepath.WalkDir(workflowRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content := mustReadContractPath(t, path)
		for _, match := range actionPattern.FindAllStringSubmatch(content, -1) {
			if strings.HasPrefix(match[1], "./") {
				continue
			}
			pin, ok := manifest.Actions[match[1]]
			if !ok {
				t.Errorf("%s uses undocumented action %s", filepath.Base(path), match[1])
				continue
			}
			seenActions[match[1]] = true
			if match[2] != pin.SHA {
				t.Errorf("%s uses %s@%s, want %s", filepath.Base(path), match[1], match[2], pin.SHA)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for action := range manifest.Actions {
		if !seenActions[action] {
			t.Errorf("runtime manifest action %s is not used", action)
		}
	}

	allWorkflows := mustReadWorkflowTree(t, workflowRoot)
	for _, floating := range []string{"ubuntu-latest", "macos-latest", "windows-latest"} {
		if strings.Contains(allWorkflows, floating) {
			t.Errorf("workflow still uses floating runner %s", floating)
		}
	}
	for _, runner := range manifest.Runners {
		if !strings.Contains(allWorkflows, runner) {
			t.Errorf("documented runner %s is not used", runner)
		}
	}

	systemdWorkflow := strings.ReplaceAll(
		mustReadContractFile(t, root, ".github/workflows/systemd-integration.yml"),
		"\\\n", "",
	)
	containerPattern := regexp.MustCompile(`pinned_image:\s*"([^"@]+)@sha256:\s*([0-9a-f]{64})"`)
	seenContainers := make(map[string]bool)
	for _, match := range containerPattern.FindAllStringSubmatch(systemdWorkflow, -1) {
		pin, ok := manifest.Containers[match[1]]
		if !ok {
			t.Errorf("systemd workflow uses undocumented container %s", match[1])
			continue
		}
		seenContainers[match[1]] = true
		if match[2] != pin.Digest {
			t.Errorf("container %s digest = %s, want %s", match[1], match[2], pin.Digest)
		}
	}
	for container := range manifest.Containers {
		if !seenContainers[container] {
			t.Errorf("runtime manifest container %s is not used", container)
		}
	}
}

func TestMaintainerAndDeploymentDocsMatchRepository(t *testing.T) {
	root := repoRoot(t)
	maintainers := mustReadContractFile(t, root, "MAINTAINERS.md")
	codeowners := mustReadContractFile(t, root, ".github/CODEOWNERS")
	if !strings.Contains(codeowners, "@omkhar") {
		t.Fatal("CODEOWNERS does not identify the repository owner")
	}
	if strings.Contains(maintainers, "does not ship `CODEOWNERS`") {
		t.Fatal("MAINTAINERS.md says that the existing CODEOWNERS file does not exist")
	}
	for _, check := range []string{"Lint and Static Analysis", "Test (ubuntu-24.04)", "CodeQL", "Dependency Review", "Analyze workflows"} {
		if !strings.Contains(maintainers, check) {
			t.Errorf("MAINTAINERS.md does not name check %q", check)
		}
	}

	required := map[string][]string{
		"deploy/systemd/README.md": {
			"systemctl is-active --quiet dns-update.timer",
			"systemctl show dns-update.service -p Result --value",
			"journalctl -u dns-update.service",
		},
		"deploy/launchd/README.md": {
			"launchctl print system/com.dns-update",
			"tail -n 50 /var/log/dns-update.log",
		},
		"deploy/windows/README.md": {
			"Get-ScheduledTaskInfo -TaskName \"dns-update\"",
			"Get-Content \"C:\\ProgramData\\dns-update\\dns-update.log\" -Tail 50",
		},
	}
	for path, fragments := range required {
		content := mustReadContractFile(t, root, path)
		for _, fragment := range fragments {
			if !strings.Contains(content, fragment) {
				t.Errorf("%s does not contain health check %q", path, fragment)
			}
		}
	}
}

func TestLimitationsReferenceCoversImplementedBoundaries(t *testing.T) {
	root := repoRoot(t)
	readme := mustReadContractFile(t, root, "README.md")
	if !strings.Contains(readme, "docs/LIMITATIONS.md") {
		t.Fatal("README.md does not link to docs/LIMITATIONS.md")
	}

	limitations := mustReadContractFile(t, root, "docs/LIMITATIONS.md")
	for _, required := range []string{
		"ASD-STE100 Simplified Technical English",
		"does not provide a distributed lock",
		"Cloudflare is the only implemented provider",
		"only A and AAAA records",
		"During normal reconciliation, the tool does not delete a record after a failed probe",
		"The explicit `-delete` mode skips probes",
		"Linux is the only platform with native packages",
		"does not install a live configuration or token",
		"does not validate live credentials",
	} {
		if !strings.Contains(limitations, required) {
			t.Errorf("docs/LIMITATIONS.md does not contain %q", required)
		}
	}
}

func mustReadWorkflowTree(t *testing.T, root string) string {
	t.Helper()
	var combined strings.Builder
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		combined.WriteString(mustReadContractPath(t, path))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return combined.String()
}

func mustReadContractFile(t *testing.T, root, relative string) string {
	t.Helper()
	return mustReadContractPath(t, filepath.Join(root, filepath.FromSlash(relative)))
}

func mustReadContractPath(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

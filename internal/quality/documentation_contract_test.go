package quality_test

import (
	"encoding/json"
	"fmt"
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

func TestGoDirectiveVersionHandlesWindowsLineEndings(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "LF", content: "module example.com/test\n\ngo 1.26.5\n"},
		{name: "CRLF", content: "module example.com/test\r\n\r\ngo 1.26.5\r\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := goDirectiveVersion(test.content); got != "1.26.5" {
				t.Fatalf("goDirectiveVersion() = %q, want 1.26.5", got)
			}
		})
	}
}

func TestWorkflowRunnersRejectUndocumentedFixedRunner(t *testing.T) {
	allowed := map[string]bool{
		"ubuntu-24.04": true,
		"windows-2025": true,
	}
	for _, test := range []struct {
		name     string
		workflow string
	}{
		{
			name: "direct runner",
			workflow: `jobs:
  test:
    runs-on: ubuntu-22.04
`,
		},
		{
			name: "matrix runner",
			workflow: `jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os:
          - ubuntu-24.04
          - macos-15
`,
		},
		{
			name: "inline matrix with fake run block values",
			workflow: `jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [macos-15]
    steps:
      - run: |
          os:
            - ubuntu-24.04
`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateWorkflowRunners("test.yml", test.workflow, allowed); err == nil {
				t.Fatal("validateWorkflowRunners accepted an undocumented runner")
			}
		})
	}
}

func TestWorkflowRunnersAcceptDocumentedFixedRunners(t *testing.T) {
	workflow := `jobs:
  direct:
    runs-on: ubuntu-24.04
  matrix:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os:
          - ubuntu-24.04
          - windows-2025
`
	allowed := map[string]bool{
		"ubuntu-24.04": true,
		"windows-2025": true,
	}
	if err := validateWorkflowRunners("test.yml", workflow, allowed); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowActionsRejectUnpinnedReference(t *testing.T) {
	workflow := `jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v7
`
	if _, err := workflowActions(workflow); err == nil {
		t.Fatal("workflowActions accepted a tag-based action reference")
	}
}

func TestNormalizeWorkflowContinuationsHandlesLineEndings(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
	}{
		{name: "LF", content: "pinned_image: image@sha256:\\\n  abc"},
		{name: "CRLF", content: "pinned_image: image@sha256:\\\r\n  abc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeWorkflowContinuations(test.content)
			if got != "pinned_image: image@sha256:  abc" {
				t.Fatalf("normalizeWorkflowContinuations() = %q", got)
			}
		})
	}
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
	goVersion := goDirectiveVersion(goMod)
	if goVersion != manifest.Go {
		t.Fatalf("go.mod version = %q, manifest version = %q", goVersion, manifest.Go)
	}

	workflowRoot := filepath.Join(root, ".github", "workflows")
	seenActions := make(map[string]bool)
	allowedRunners := make(map[string]bool, len(manifest.Runners))
	for _, runner := range manifest.Runners {
		allowedRunners[runner] = true
	}
	seenRunners := make(map[string]bool)
	err = filepath.WalkDir(workflowRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content := mustReadContractPath(t, path)
		actions, actionErr := workflowActions(content)
		if actionErr != nil {
			t.Errorf("%s: %v", filepath.Base(path), actionErr)
		}
		for _, action := range actions {
			pin, ok := manifest.Actions[action.name]
			if !ok {
				t.Errorf("%s uses undocumented action %s", filepath.Base(path), action.name)
				continue
			}
			seenActions[action.name] = true
			if action.sha != pin.SHA {
				t.Errorf("%s uses %s@%s, want %s", filepath.Base(path), action.name, action.sha, pin.SHA)
			}
		}
		runners, runnerErr := workflowRunners(content)
		if runnerErr != nil {
			t.Errorf("%s: %v", filepath.Base(path), runnerErr)
		}
		for _, runner := range runners {
			if !allowedRunners[runner] {
				t.Errorf("%s uses undocumented runner %s", filepath.Base(path), runner)
				continue
			}
			seenRunners[runner] = true
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

	for _, runner := range manifest.Runners {
		if !seenRunners[runner] {
			t.Errorf("documented runner %s is not used", runner)
		}
	}

	systemdWorkflow := normalizeWorkflowContinuations(
		mustReadContractFile(t, root, ".github/workflows/systemd-integration.yml"),
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

func goDirectiveVersion(goMod string) string {
	match := regexp.MustCompile(`(?m)^go[ \t]+([^ \t\r\n]+)[ \t]*\r?$`).FindStringSubmatch(goMod)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func normalizeWorkflowContinuations(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\\\n", "")
}

type workflowAction struct {
	name string
	sha  string
}

func workflowActions(content string) ([]workflowAction, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	pattern := regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?uses:[ \t]*([^\r\n]+)$`)
	var actions []workflowAction
	for _, match := range pattern.FindAllStringSubmatch(content, -1) {
		reference := workflowScalar(match[1])
		if strings.HasPrefix(reference, "./") {
			continue
		}
		separator := strings.LastIndexByte(reference, '@')
		if separator < 1 {
			return nil, fmt.Errorf("external action reference %q has no commit revision", reference)
		}
		name, sha := reference[:separator], reference[separator+1:]
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
			return nil, fmt.Errorf("external action %s uses non-immutable revision %q", name, sha)
		}
		actions = append(actions, workflowAction{name: name, sha: sha})
	}
	return actions, nil
}

func validateWorkflowRunners(path, content string, allowed map[string]bool) error {
	runners, err := workflowRunners(content)
	if err != nil {
		return err
	}
	for _, runner := range runners {
		if !allowed[runner] {
			return fmt.Errorf("%s uses undocumented runner %s", path, runner)
		}
	}
	return nil
}

func workflowRunners(content string) ([]string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	type jobRuntime struct {
		name        string
		runsOn      string
		matrixOS    []string
		sawMatrix   bool
		sawMatrixOS bool
	}

	lines := strings.Split(content, "\n")
	jobPattern := regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*):[ \t]*(?:#.*)?$`)
	jobs := make([]jobRuntime, 0)
	inJobs := false
	currentJob := -1
	inStrategy := false
	inMatrix := false
	inMatrixOS := false

	for lineNumber, line := range lines {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if strings.Contains(leading, "\t") {
			return nil, fmt.Errorf("line %d uses a tab for indentation", lineNumber+1)
		}
		indent := len(leading)
		trimmed := strings.TrimSpace(line)

		if inMatrixOS && indent <= 8 {
			inMatrixOS = false
		}
		if inMatrix && indent <= 6 {
			inMatrix = false
		}
		if inStrategy && indent <= 4 {
			inStrategy = false
		}

		if indent == 0 {
			inJobs = trimmed == "jobs:"
			currentJob = -1
			continue
		}
		if !inJobs {
			continue
		}
		if indent == 2 {
			match := jobPattern.FindStringSubmatch(trimmed)
			if len(match) != 2 {
				return nil, fmt.Errorf("line %d has an unsupported job declaration", lineNumber+1)
			}
			jobs = append(jobs, jobRuntime{name: match[1]})
			currentJob = len(jobs) - 1
			continue
		}
		if currentJob < 0 {
			continue
		}

		if indent == 4 {
			if strings.HasPrefix(trimmed, "runs-on:") {
				if jobs[currentJob].runsOn != "" {
					return nil, fmt.Errorf("job %s has more than one runs-on value", jobs[currentJob].name)
				}
				jobs[currentJob].runsOn = workflowScalar(strings.TrimPrefix(trimmed, "runs-on:"))
				if jobs[currentJob].runsOn == "" {
					return nil, fmt.Errorf("job %s has an empty runs-on value", jobs[currentJob].name)
				}
				continue
			}
			if trimmed == "strategy:" {
				inStrategy = true
			}
			continue
		}
		if indent == 6 && inStrategy && trimmed == "matrix:" {
			inMatrix = true
			jobs[currentJob].sawMatrix = true
			continue
		}
		if indent == 8 && inMatrix && strings.HasPrefix(trimmed, "os:") {
			if jobs[currentJob].sawMatrixOS {
				return nil, fmt.Errorf("job %s has more than one matrix.os definition", jobs[currentJob].name)
			}
			if workflowScalar(strings.TrimPrefix(trimmed, "os:")) != "" {
				return nil, fmt.Errorf("job %s matrix.os must use a fixed block list", jobs[currentJob].name)
			}
			jobs[currentJob].sawMatrixOS = true
			inMatrixOS = true
			continue
		}
		if inMatrixOS {
			if indent != 10 || !strings.HasPrefix(trimmed, "- ") {
				return nil, fmt.Errorf("job %s matrix.os contains an unsupported value on line %d", jobs[currentJob].name, lineNumber+1)
			}
			value := workflowScalar(strings.TrimPrefix(trimmed, "- "))
			if value == "" || strings.Contains(value, "${{") {
				return nil, fmt.Errorf("job %s matrix.os contains non-fixed runner %q", jobs[currentJob].name, value)
			}
			jobs[currentJob].matrixOS = append(jobs[currentJob].matrixOS, value)
		}
	}

	var runners []string
	for _, job := range jobs {
		if job.runsOn == "" {
			continue
		}
		if !strings.Contains(job.runsOn, "${{") {
			runners = append(runners, job.runsOn)
			continue
		}
		if job.runsOn != "${{ matrix.os }}" {
			return nil, fmt.Errorf("job %s has unsupported runs-on expression %q", job.name, job.runsOn)
		}
		if !job.sawMatrix || !job.sawMatrixOS || len(job.matrixOS) == 0 {
			return nil, fmt.Errorf("job %s uses matrix.os, but matrix.os has no fixed block values", job.name)
		}
		runners = append(runners, job.matrixOS...)
	}
	return runners, nil
}

func workflowScalar(value string) string {
	value = strings.TrimSpace(value)
	comment := strings.Index(value, " #")
	if tabComment := strings.Index(value, "\t#"); tabComment >= 0 && (comment < 0 || tabComment < comment) {
		comment = tabComment
	}
	if comment >= 0 {
		value = strings.TrimSpace(value[:comment])
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	return value
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
	for _, check := range []string{
		"CI / Lint and Static Analysis",
		"CI / Test (ubuntu-24.04)",
		"CI / Test (macos-26)",
		"CI / Test (windows-2025)",
		"CodeQL / CodeQL",
		"Dependency Review / Dependency Review",
		"zizmor / Analyze workflows",
	} {
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

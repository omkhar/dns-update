package quality_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type pinnedRuntime struct {
	SHA string `json:"sha"`
}

type pinnedContainer struct {
	Digest string `json:"digest"`
}

type runtimeManifest struct {
	Go         string                     `json:"go"`
	Runners    []string                   `json:"runners"`
	Actions    map[string]pinnedRuntime   `json:"actions"`
	Containers map[string]pinnedContainer `json:"containers"`
	Tools      map[string]string          `json:"tools"`
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

func TestWorkflowContractRejectsUnsupportedYAMLForms(t *testing.T) {
	sha := "3d3c42e5aac5ba805825da76410c181273ba90b1"
	for _, test := range []struct {
		name     string
		workflow string
	}{
		{
			name:     "quoted uses key",
			workflow: "jobs:\n  test:\n    steps:\n      - \"uses\": actions/checkout@v7\n",
		},
		{
			name:     "spaced uses key",
			workflow: "jobs:\n  test:\n    steps:\n      - uses : actions/checkout@v7\n",
		},
		{
			name:     "flow mapping step",
			workflow: "jobs:\n  test:\n    steps:\n      - { uses: actions/checkout@v7 }\n",
		},
		{
			name:     "mapping alias",
			workflow: fmt.Sprintf("jobs:\n  test:\n    steps:\n      - &checkout\n        uses: actions/checkout@%s\n      - *checkout\n", sha),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := workflowActions(test.workflow); err == nil {
				t.Fatal("workflowActions accepted unsupported YAML")
			}
		})
	}

	quotedRunner := "jobs:\n  test:\n    \"runs-on\": macos-15\n"
	if err := validateWorkflowRunners("test.yml", quotedRunner, map[string]bool{"ubuntu-24.04": true}); err == nil {
		t.Fatal("validateWorkflowRunners accepted a quoted undocumented runner key")
	}

	wideIndentation := "jobs:\n    test:\n        runs-on: macos-15\n"
	if err := validateWorkflowRunners("test.yml", wideIndentation, map[string]bool{"ubuntu-24.04": true}); err == nil {
		t.Fatal("validateWorkflowRunners accepted an undocumented runner with valid wide indentation")
	}

	taggedJobs := "jobs: !!map\n  test:\n    runs-on: macos-15\n"
	if err := validateWorkflowRunners("test.yml", taggedJobs, map[string]bool{"ubuntu-24.04": true}); err == nil {
		t.Fatal("validateWorkflowRunners accepted an explicit YAML tag that hides an undocumented runner")
	}
}

func TestWorkflowContractIgnoresBlockScalarText(t *testing.T) {
	workflow := `jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          uses: actions/checkout@v7
          "runs-on": macos-15
`
	actions, err := workflowActions(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("workflowActions() returned %d actions, want 0", len(actions))
	}
	if err := validateWorkflowRunners("test.yml", workflow, map[string]bool{"ubuntu-24.04": true}); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeManifestRejectsUnvalidatedFields(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{
			name: "action version",
			data: `{"go":"1.26.5","runners":[],"actions":{"actions/checkout":{"sha":"3d3c42e5aac5ba805825da76410c181273ba90b1","version":"v0.0.0-peer-mutant"}},"containers":{}}`,
		},
		{
			name: "container tag",
			data: `{"go":"1.26.5","runners":[],"actions":{},"containers":{"fedora:44":{"digest":"6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898","tag":"peer-mutant"}}}`,
		},
		{
			name: "duplicate SHA",
			data: `{"go":"1.26.5","runners":[],"actions":{"actions/checkout":{"sha":"0000000000000000000000000000000000000000","sha":"3d3c42e5aac5ba805825da76410c181273ba90b1"}},"containers":{}}`,
		},
		{
			name: "duplicate runner",
			data: `{"go":"1.26.5","runners":["ubuntu-24.04","ubuntu-24.04"],"actions":{},"containers":{}}`,
		},
		{
			name: "invalid action SHA",
			data: `{"go":"1.26.5","runners":[],"actions":{"actions/checkout":{"sha":"v7"}},"containers":{}}`,
		},
		{
			name: "invalid container digest",
			data: `{"go":"1.26.5","runners":[],"actions":{},"containers":{"fedora:44":{"digest":"latest"}}}`,
		},
		{
			name: "empty tool version",
			data: `{"go":"1.26.5","runners":[],"actions":{},"containers":{},"tools":{"actionlint":""}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeRuntimeManifest(strings.NewReader(test.data)); err == nil {
				t.Fatal("decodeRuntimeManifest accepted an unvalidated field")
			}
		})
	}
}

func TestWorkflowToolsRejectConflictingVersions(t *testing.T) {
	workflow := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          go install github.com/rhysd/actionlint/cmd/actionlint@v0.0.0-peer-mutant
`
	if _, err := workflowTools(workflow); err == nil {
		t.Fatal("workflowTools accepted conflicting tool versions")
	}
}

func TestWorkflowToolsRejectNonCommandDecoys(t *testing.T) {
	workflow := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          # go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          echo "go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12"
          # yamllint==1.38.0
          echo yamllint==1.38.0
`
	if _, err := workflowTools(workflow); err == nil {
		t.Fatal("workflowTools accepted a non-command tool-version decoy")
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

func TestWorkflowContainersRejectMismatchedFloatingImage(t *testing.T) {
	workflow := `jobs:
  systemd:
    strategy:
      matrix:
        include:
          - floating_image: "fedora:peer-mutant"
            pinned_image: "fedora:44@sha256:6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898"
`
	if _, err := workflowContainers(workflow); err == nil {
		t.Fatal("workflowContainers accepted a floating image that does not match its pinned image")
	}
}

func TestRuntimeManifestMatchesRepository(t *testing.T) {
	root := repoRoot(t)
	manifestFile, err := os.Open(filepath.Join(root, "docs", "runtime-versions.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manifestFile.Close() })
	manifest, err := decodeRuntimeManifest(manifestFile)
	if err != nil {
		t.Fatal(err)
	}
	runtimeDoc := mustReadContractFile(t, root, "docs/RUNTIME.md")
	if !strings.Contains(runtimeDoc, "Use Go "+manifest.Go+" to build, test, and release `dns-update`.") {
		t.Errorf("docs/RUNTIME.md does not state Go %s", manifest.Go)
	}
	for tool, version := range manifest.Tools {
		name := tool
		if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
			name = name[slash+1:]
		}
		line := "- " + name + " " + strings.TrimPrefix(version, "v")
		if strings.Count(runtimeDoc, line) != 1 {
			t.Errorf("docs/RUNTIME.md must contain %q exactly once", line)
		}
	}

	goMod := mustReadContractFile(t, root, "go.mod")
	goVersion := goDirectiveVersion(goMod)
	if goVersion != manifest.Go {
		t.Fatalf("go.mod version = %q, manifest version = %q", goVersion, manifest.Go)
	}

	workflowRoot := filepath.Join(root, ".github", "workflows")
	seenActions := make(map[string]bool)
	seenTools := make(map[string]bool)
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
		installedTools, toolErr := workflowTools(content)
		if toolErr != nil {
			t.Errorf("%s: %v", filepath.Base(path), toolErr)
		}
		for tool, version := range installedTools {
			want, ok := manifest.Tools[tool]
			if !ok {
				t.Errorf("%s installs undocumented tool %s", filepath.Base(path), tool)
				continue
			}
			seenTools[tool] = true
			if version != want {
				t.Errorf("%s installs %s@%s, want %s", filepath.Base(path), tool, version, want)
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
	for tool := range manifest.Tools {
		if !seenTools[tool] {
			t.Errorf("runtime manifest tool %s is not installed", tool)
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
	containers, containerErr := workflowContainers(systemdWorkflow)
	if containerErr != nil {
		t.Fatal(containerErr)
	}
	seenContainers := make(map[string]bool)
	for _, container := range containers {
		pin, ok := manifest.Containers[container.name]
		if !ok {
			t.Errorf("systemd workflow uses undocumented container %s", container.name)
			continue
		}
		seenContainers[container.name] = true
		if container.digest != pin.Digest {
			t.Errorf("container %s digest = %s, want %s", container.name, container.digest, pin.Digest)
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

func decodeRuntimeManifest(reader io.Reader) (runtimeManifest, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return runtimeManifest{}, err
	}
	if err := validateUniqueJSONKeys(data); err != nil {
		return runtimeManifest{}, err
	}
	var manifest runtimeManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return runtimeManifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return runtimeManifest{}, fmt.Errorf("runtime manifest contains more than one JSON value")
		}
		return runtimeManifest{}, err
	}
	if err := validateRuntimeManifest(manifest); err != nil {
		return runtimeManifest{}, err
	}
	return manifest, nil
}

func validateRuntimeManifest(manifest runtimeManifest) error {
	if manifest.Go == "" {
		return fmt.Errorf("runtime manifest has no Go version")
	}
	seenRunners := make(map[string]bool, len(manifest.Runners))
	for _, runner := range manifest.Runners {
		if runner == "" {
			return fmt.Errorf("runtime manifest has an empty runner")
		}
		if seenRunners[runner] {
			return fmt.Errorf("runtime manifest runner %s occurs more than once", runner)
		}
		seenRunners[runner] = true
	}
	shaPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for action, pin := range manifest.Actions {
		if action == "" || !shaPattern.MatchString(pin.SHA) {
			return fmt.Errorf("runtime manifest action %q has an invalid commit SHA", action)
		}
	}
	digestPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for container, pin := range manifest.Containers {
		if container == "" || !digestPattern.MatchString(pin.Digest) {
			return fmt.Errorf("runtime manifest container %q has an invalid digest", container)
		}
	}
	for tool, version := range manifest.Tools {
		if tool == "" || version == "" {
			return fmt.Errorf("runtime manifest tool %q has an invalid version", tool)
		}
	}
	return nil
}

func validateUniqueJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := validateUniqueJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("runtime manifest contains more than one JSON value")
		}
		return err
	}
	return nil
}

func validateUniqueJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("runtime manifest contains a non-string object key")
			}
			if seen[key] {
				return fmt.Errorf("runtime manifest contains duplicate key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return fmt.Errorf("runtime manifest object has no closing delimiter")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := validateUniqueJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return fmt.Errorf("runtime manifest array has no closing delimiter")
		}
	default:
		return fmt.Errorf("runtime manifest contains an unexpected delimiter %q", delimiter)
	}
	return nil
}

func normalizeWorkflowContinuations(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	return strings.ReplaceAll(content, "\\\n", "")
}

type workflowContainer struct {
	name   string
	digest string
}

func workflowContainers(content string) ([]workflowContainer, error) {
	lines, err := workflowStructuralLines(normalizeWorkflowContinuations(content))
	if err != nil {
		return nil, err
	}
	pendingFloating := ""
	seen := make(map[string]bool)
	var containers []workflowContainer
	for _, line := range lines {
		key, value, ok := canonicalWorkflowMapping(line.content)
		if !ok {
			continue
		}
		value = workflowScalar(value)
		switch key {
		case "floating_image":
			if pendingFloating != "" {
				return nil, fmt.Errorf("line %d starts a new floating image before its pinned image", line.number)
			}
			if value == "" || strings.ContainsAny(value, `\@`) {
				return nil, fmt.Errorf("line %d has an invalid floating image", line.number)
			}
			pendingFloating = value
		case "pinned_image":
			if pendingFloating == "" {
				return nil, fmt.Errorf("line %d has a pinned image without a floating image", line.number)
			}
			match := regexp.MustCompile(`^([^@]+)@sha256:[ \t]*([0-9a-f]{64})$`).FindStringSubmatch(value)
			if len(match) != 3 {
				return nil, fmt.Errorf("line %d has an invalid pinned image", line.number)
			}
			if match[1] != pendingFloating {
				return nil, fmt.Errorf("floating image %s does not match pinned image %s", pendingFloating, match[1])
			}
			if seen[pendingFloating] {
				return nil, fmt.Errorf("container %s occurs more than once", pendingFloating)
			}
			seen[pendingFloating] = true
			containers = append(containers, workflowContainer{name: pendingFloating, digest: match[2]})
			pendingFloating = ""
		}
	}
	if pendingFloating != "" {
		return nil, fmt.Errorf("floating image %s has no pinned image", pendingFloating)
	}
	return containers, nil
}

type workflowAction struct {
	name string
	sha  string
}

func workflowActions(content string) ([]workflowAction, error) {
	lines, err := workflowStructuralLines(content)
	if err != nil {
		return nil, err
	}
	var actions []workflowAction
	for _, line := range lines {
		key, value, ok := canonicalWorkflowMapping(line.content)
		if !ok || key != "uses" {
			continue
		}
		reference := workflowScalar(value)
		if strings.Contains(reference, `\`) {
			return nil, fmt.Errorf("line %d uses an escaped action reference", line.number)
		}
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

func workflowTools(content string) (map[string]string, error) {
	content = normalizeWorkflowContinuations(content)
	tools := make(map[string]string)
	goInstallPattern := regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_]*=(?:"[^"]*"|'[^']*'|[^ \t]+)[ \t]+)*go[ \t]+install[ \t]+([A-Za-z0-9._/-]+)@([A-Za-z0-9.+_-]+)$`)
	yamllintPattern := regexp.MustCompile(`^(?:"[^"]*pip"|[^ \t]*pip)[ \t]+install(?:[ \t]+--[A-Za-z0-9-]+(?:=[^ \t]+)?)*[ \t]+yamllint==([A-Za-z0-9.+_-]+)$`)
	for lineNumber, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if match := goInstallPattern.FindStringSubmatch(line); len(match) == 3 {
			if oldVersion, exists := tools[match[1]]; exists && oldVersion != match[2] {
				return nil, fmt.Errorf("tool %s has conflicting versions %s and %s", match[1], oldVersion, match[2])
			}
			tools[match[1]] = match[2]
			continue
		}
		if strings.Contains(line, "go install") {
			return nil, fmt.Errorf("line %d has an unsupported go install command", lineNumber+1)
		}
		if match := yamllintPattern.FindStringSubmatch(line); len(match) == 2 {
			if oldVersion, exists := tools["yamllint"]; exists && oldVersion != match[1] {
				return nil, fmt.Errorf("tool yamllint has conflicting versions %s and %s", oldVersion, match[1])
			}
			tools["yamllint"] = match[1]
			continue
		}
		if strings.Contains(line, "yamllint==") {
			return nil, fmt.Errorf("line %d has an unsupported yamllint install command", lineNumber+1)
		}
	}
	return tools, nil
}

type workflowStructuralLine struct {
	number  int
	indent  int
	content string
}

var (
	canonicalWorkflowKey = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*):[ \t]*(.*)$`)
	quotedOnKey          = regexp.MustCompile(`^['"]on['"]:[ \t]*(.*)$`)
	blockScalarValue     = regexp.MustCompile(`^[|>][0-9+-]*$`)
)

func workflowStructuralLines(content string) ([]workflowStructuralLine, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	blockIndent := -1
	var result []workflowStructuralLine
	for index, raw := range strings.Split(content, "\n") {
		number := index + 1
		if strings.ContainsRune(raw, '\t') {
			return nil, fmt.Errorf("line %d uses a tab for indentation", number)
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		content := raw[indent:]
		if blockIndent >= 0 {
			if strings.TrimSpace(raw) == "" || indent > blockIndent {
				continue
			}
			blockIndent = -1
		}
		if content == "" || strings.HasPrefix(content, "#") {
			continue
		}
		if err := validateCanonicalWorkflowLine(indent, content); err != nil {
			return nil, fmt.Errorf("line %d: %w", number, err)
		}
		result = append(result, workflowStructuralLine{number: number, indent: indent, content: content})
		if workflowBlockScalarHeader(content) {
			blockIndent = indent
		}
	}
	return result, nil
}

func validateCanonicalWorkflowLine(indent int, content string) error {
	if content == "---" {
		return nil
	}
	if indent == 0 && quotedOnKey.MatchString(content) {
		return nil
	}

	value := content
	sequence := false
	if strings.HasPrefix(value, "- ") {
		sequence = true
		value = strings.TrimSpace(strings.TrimPrefix(value, "- "))
		if value == "" {
			return fmt.Errorf("empty sequence entries are not supported")
		}
		if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
			return fmt.Errorf("flow-style sequence mappings are not supported")
		}
		if strings.HasPrefix(value, "&") || strings.HasPrefix(value, "*") {
			return fmt.Errorf("YAML anchors and aliases are not supported")
		}
	} else if strings.HasPrefix(value, "-") {
		return fmt.Errorf("sequence entries must use a space after the dash")
	}

	if _, mappingValue, ok := canonicalWorkflowMapping(value); ok {
		mappingValue = strings.TrimSpace(mappingValue)
		if strings.HasPrefix(mappingValue, "&") || strings.HasPrefix(mappingValue, "*") {
			return fmt.Errorf("YAML anchors and aliases are not supported")
		}
		if strings.HasPrefix(mappingValue, "!") {
			return fmt.Errorf("explicit YAML tags are not supported")
		}
		if strings.HasPrefix(mappingValue, "[") ||
			(strings.HasPrefix(mappingValue, "{") && mappingValue != "{}") {
			return fmt.Errorf("non-empty flow-style values are not supported")
		}
		return nil
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("mapping keys must use an unquoted key with no space before the colon")
	}
	if !sequence && indent == 0 {
		return fmt.Errorf("unsupported root declaration")
	}
	return nil
}

func canonicalWorkflowMapping(content string) (string, string, bool) {
	if strings.HasPrefix(content, "- ") {
		content = strings.TrimSpace(strings.TrimPrefix(content, "- "))
	}
	match := canonicalWorkflowKey.FindStringSubmatch(content)
	if len(match) != 3 {
		return "", "", false
	}
	return match[1], match[2], true
}

func workflowBlockScalarHeader(content string) bool {
	_, value, ok := canonicalWorkflowMapping(content)
	if !ok {
		return false
	}
	if comment := strings.Index(value, " #"); comment >= 0 {
		value = value[:comment]
	}
	return blockScalarValue.MatchString(strings.TrimSpace(value))
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
	lines, err := workflowStructuralLines(content)
	if err != nil {
		return nil, err
	}
	type mappingFrame struct {
		indent int
		key    string
	}
	type jobRuntime struct {
		name        string
		runsOn      string
		matrixOS    []string
		sawMatrix   bool
		sawMatrixOS bool
	}

	var stack []mappingFrame
	var jobs []jobRuntime
	jobIndexes := make(map[string]int)
	for _, line := range lines {
		for len(stack) > 0 && stack[len(stack)-1].indent >= line.indent {
			stack = stack[:len(stack)-1]
		}

		key, rawValue, isMapping := canonicalWorkflowMapping(line.content)
		if !isMapping {
			if len(stack) == 5 && stack[0].key == "jobs" && stack[2].key == "strategy" &&
				stack[3].key == "matrix" && stack[4].key == "os" {
				value := strings.TrimSpace(line.content)
				if !strings.HasPrefix(value, "- ") {
					return nil, fmt.Errorf("job %s matrix.os contains an unsupported value on line %d", stack[1].key, line.number)
				}
				value = workflowScalar(strings.TrimPrefix(value, "- "))
				if value == "" || strings.Contains(value, "${{") {
					return nil, fmt.Errorf("job %s matrix.os contains non-fixed runner %q", stack[1].key, value)
				}
				job := &jobs[jobIndexes[stack[1].key]]
				job.matrixOS = append(job.matrixOS, value)
			}
			continue
		}

		path := make([]string, 0, len(stack)+1)
		for _, frame := range stack {
			path = append(path, frame.key)
		}
		path = append(path, key)
		value := workflowScalar(rawValue)

		switch {
		case len(path) == 2 && path[0] == "jobs":
			if _, exists := jobIndexes[key]; exists {
				return nil, fmt.Errorf("job %s occurs more than once", key)
			}
			jobIndexes[key] = len(jobs)
			jobs = append(jobs, jobRuntime{name: key})
		case len(path) == 3 && path[0] == "jobs" && key == "runs-on":
			job := &jobs[jobIndexes[path[1]]]
			if job.runsOn != "" {
				return nil, fmt.Errorf("job %s has more than one runs-on value", job.name)
			}
			if value == "" {
				return nil, fmt.Errorf("job %s has an empty runs-on value", job.name)
			}
			job.runsOn = value
		case len(path) == 4 && path[0] == "jobs" && path[2] == "strategy" && key == "matrix":
			jobs[jobIndexes[path[1]]].sawMatrix = true
		case len(path) == 5 && path[0] == "jobs" && path[2] == "strategy" &&
			path[3] == "matrix" && key == "os":
			job := &jobs[jobIndexes[path[1]]]
			if job.sawMatrixOS {
				return nil, fmt.Errorf("job %s has more than one matrix.os definition", job.name)
			}
			if value != "" {
				return nil, fmt.Errorf("job %s matrix.os must use a fixed block list", job.name)
			}
			job.sawMatrixOS = true
		}

		trimmedValue := strings.TrimSpace(rawValue)
		if trimmedValue == "" || strings.HasPrefix(trimmedValue, "#") {
			stack = append(stack, mappingFrame{indent: line.indent, key: key})
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

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

type pinnedContainer struct {
	Digest string `json:"digest"`
}

// runtimeManifest does not copy values that Dependabot changes. go.mod holds
// the Go version, and each workflow "uses:" line holds its action commit SHA.
// The tests read those values from their source files.
type runtimeManifest struct {
	Runners    []string                   `json:"runners"`
	Actions    []string                   `json:"actions"`
	Containers map[string]pinnedContainer `json:"containers"`
	Tools      map[string]string          `json:"tools"`
}

// goVersionResolverStep is the canonical step that each job runs before
// actions/setup-go. It turns the go.mod "go" version into a SemVer tilde range,
// so setup-go with check-latest installs the newest patch release of that
// minor version. A new Go patch release then needs no repository change.
const goVersionResolverStep = `      - name: Resolve Go version
        id: go-version
        shell: bash
        run: |
          go_version="$(sed -n 's/^go \([0-9][0-9.]*\)[[:space:]]*$/\1/p' go.mod)"
          test -n "$go_version"
          echo "range=~$go_version" >> "$GITHUB_OUTPUT"
`

var goDirectiveVersionPattern = regexp.MustCompile(`^1\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?$`)

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

func TestWorkflowRunnersRejectMatrixInclude(t *testing.T) {
	workflow := `jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os:
          - ubuntu-24.04
        include:
          - os: ubuntu-22.04
`
	if err := validateWorkflowRunners("test.yml", workflow, map[string]bool{"ubuntu-24.04": true}); err == nil {
		t.Fatal("validateWorkflowRunners accepted an unmodeled matrix.include runner")
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
			name: "copied Go version",
			data: `{"go":"1.26.5","runners":[],"actions":[],"containers":{}}`,
		},
		{
			name: "copied action SHA",
			data: `{"runners":[],"actions":{"actions/checkout":{"sha":"3d3c42e5aac5ba805825da76410c181273ba90b1"}},"containers":{}}`,
		},
		{
			name: "action with revision",
			data: `{"runners":[],"actions":["actions/checkout@v7"],"containers":{}}`,
		},
		{
			name: "duplicate action",
			data: `{"runners":[],"actions":["actions/checkout","actions/checkout"],"containers":{}}`,
		},
		{
			name: "empty action",
			data: `{"runners":[],"actions":[""],"containers":{}}`,
		},
		{
			name: "container tag",
			data: `{"runners":[],"actions":[],"containers":{"fedora:44":{"digest":"6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898","tag":"peer-mutant"}}}`,
		},
		{
			name: "duplicate digest",
			data: `{"runners":[],"actions":[],"containers":{"fedora:44":{"digest":"0000000000000000000000000000000000000000000000000000000000000000","digest":"6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898"}}}`,
		},
		{
			name: "duplicate runner",
			data: `{"runners":["ubuntu-24.04","ubuntu-24.04"],"actions":[],"containers":{}}`,
		},
		{
			name: "invalid container digest",
			data: `{"runners":[],"actions":[],"containers":{"fedora:44":{"digest":"latest"}}}`,
		},
		{
			name: "empty tool version",
			data: `{"runners":[],"actions":[],"containers":{},"tools":{"actionlint":""}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeRuntimeManifest(strings.NewReader(test.data)); err == nil {
				t.Fatal("decodeRuntimeManifest accepted an unvalidated field")
			}
		})
	}
}

func TestWorkflowActionPinsRejectSplitRepositoryPins(t *testing.T) {
	sha := "e4fba868fa4b1b91e1fdab776edc8cfbe6e9fb81"
	otherSHA := "3d3c42e5aac5ba805825da76410c181273ba90b1"
	valid := []workflowAction{
		{name: "github/codeql-action/init", sha: sha},
		{name: "github/codeql-action/analyze", sha: sha},
		{name: "actions/checkout", sha: otherSHA},
		{name: "actions/checkout", sha: otherSHA},
	}
	if err := validateWorkflowActionPins(valid); err != nil {
		t.Fatal(err)
	}
	for name, actions := range map[string][]workflowAction{
		"two SHAs for one action": {
			{name: "actions/checkout", sha: otherSHA},
			{name: "actions/checkout", sha: sha},
		},
		"two SHAs for one repository": {
			{name: "github/codeql-action/init", sha: sha},
			{name: "github/codeql-action/analyze", sha: otherSHA},
		},
		"non-SHA revision": {
			{name: "actions/checkout", sha: "v7"},
		},
		"no repository": {
			{name: "checkout", sha: sha},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateWorkflowActionPins(actions); err == nil {
				t.Fatal("validateWorkflowActionPins accepted an unpinned or split action pin")
			}
		})
	}
}

func TestWorkflowGoSetupRequiresLatestPatchOfGoModMinor(t *testing.T) {
	setup := func(with string) string {
		return "jobs:\n  test:\n    runs-on: ubuntu-24.04\n    steps:\n" + goVersionResolverStep +
			"      - name: Set up Go\n        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e  # v7.0.0\n        with:\n" + with
	}
	valid := setup("          go-version: ${{ steps.go-version.outputs.range }}\n          check-latest: true\n")
	if err := validateWorkflowGoSetup(valid); err != nil {
		t.Fatal(err)
	}
	for name, workflow := range map[string]string{
		"exact go.mod version": setup("          go-version-file: go.mod\n"),
		"no check-latest":      setup("          go-version: ${{ steps.go-version.outputs.range }}\n"),
		"fixed version":        setup("          go-version: 1.26.10\n          check-latest: true\n"),
		"both inputs": setup("          go-version: ${{ steps.go-version.outputs.range }}\n" +
			"          check-latest: true\n          go-version-file: go.mod\n"),
		"no resolver step": "jobs:\n  test:\n    runs-on: ubuntu-24.04\n    steps:\n" +
			"      - name: Set up Go\n        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e  # v7.0.0\n        with:\n" +
			"          go-version: ${{ steps.go-version.outputs.range }}\n          check-latest: true\n",
		"changed resolver step": strings.Replace(valid, `echo "range=~$go_version"`, `echo "range=$go_version"`, 1),
		"resolver in another job": "jobs:\n  resolve:\n    runs-on: ubuntu-24.04\n    steps:\n" + goVersionResolverStep +
			"  test:\n    runs-on: ubuntu-24.04\n    steps:\n" +
			"      - name: Set up Go\n        uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e  # v7.0.0\n        with:\n" +
			"          go-version: ${{ steps.go-version.outputs.range }}\n          check-latest: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateWorkflowGoSetup(workflow); err == nil {
				t.Fatal("validateWorkflowGoSetup accepted a Go setup that does not track the latest patch release")
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

func TestWorkflowToolReferencesRejectDeadCompliantInstall(t *testing.T) {
	workflows := `if false; then
  go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
fi
go_subcommand=install
go "${go_subcommand}" github.com/rhysd/actionlint/cmd/actionlint@v1.7.11
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted a dead compliant install and an older executable install")
	}
}

func TestWorkflowToolReferencesRejectSplitDynamicInstall(t *testing.T) {
	workflows := `if false; then
  go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
fi
go_subcommand=install
tool_root=github.com/rhysd/actionlint/cmd
tool_name=actionlint
go "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted a dead compliant install and a split dynamic executable install")
	}
}

func TestWorkflowToolReferencesRejectFunctionDecoyAndCommandIndirection(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          install_manifest_actionlint() {
            GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          }
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          GOBIN="$RUNNER_TEMP/bin" command go "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted an uncalled function decoy and command indirection")
	}
}

func TestWorkflowToolReferencesRejectDynamicExecutableIndirection(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          go_binary=go
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          GOBIN="$RUNNER_TEMP/bin" "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted a dynamically selected Go executable")
	}
}

func TestWorkflowToolReferencesRejectCommandWrappedDynamicExecutable(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          go_binary=go
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          command "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted command-wrapped dynamic executable selection")
	}
}

func TestWorkflowToolReferencesRejectTimeWrappedDynamicExecutable(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          go_binary=go
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          time "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted time-wrapped dynamic executable selection")
	}
}

func TestWorkflowToolReferencesRejectHyphenatedFunctionDecoy(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          function install-manifest-actionlint-v1 {
            GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          }
          go_binary=go
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          command "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted a hyphenated function decoy")
	}
}

func TestWorkflowToolReferencesRejectNestedCommandSubstitution(t *testing.T) {
	workflows := `jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
          echo "$(go_binary=go; go_subcommand=install; tool_root=github.com/rhysd/actionlint/cmd; tool_name=actionlint; command "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11")" >> "$GITHUB_PATH"
`
	tools := map[string]string{"github.com/rhysd/actionlint/cmd/actionlint": "v1.7.12"}
	if err := validateWorkflowToolReferences(tools, workflows); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted an older install in nested command substitution")
	}
}

func TestWorkflowToolReferencesRejectNonexecutingRunDecoy(t *testing.T) {
	workflow := `name: runtime decoy replay

on:
  workflow_dispatch:

env:
  run: |
    GOBIN="$RUNNER_TEMP/bin" go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
    GOBIN="$RUNNER_TEMP/bin" go install golang.org/x/vuln/cmd/govulncheck@v1.6.0
    GOBIN="$RUNNER_TEMP/bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
    echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"
    sudo apt-get update
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends shellcheck

jobs:
  lint:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          go_binary=go
          go_subcommand=install
          tool_root=github.com/rhysd/actionlint/cmd
          tool_name=actionlint
          GOBIN="$RUNNER_TEMP/bin" "${go_binary}" "${go_subcommand}" "${tool_root}/${tool_name}@v1.7.11"
`
	tools := map[string]string{
		"github.com/rhysd/actionlint/cmd/actionlint":             "v1.7.12",
		"golang.org/x/vuln/cmd/govulncheck":                      "v1.6.0",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint": "v2.12.2",
	}
	if err := validateWorkflowToolReferences(tools, workflow); err == nil {
		t.Fatal("validateWorkflowToolReferences accepted a non-executing env.run decoy")
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

func TestWorkflowRuntimeLimitationsDocumented(t *testing.T) {
	limitations := mustReadContractFile(t, repoRoot(t), "docs/LIMITATIONS.md")
	for _, statement := range []string{
		"The workflow runtime check reads a restricted YAML form.",
		"It reads external actions only from jobs and steps that GitHub Actions can run.",
		"It reads integration container pins only from the systemd matrix entries.",
		"Each tracked tool installation must use its exact canonical run block.",
		"It cannot detect a tracked tool name that a workflow builds from separate string fragments.",
	} {
		if !strings.Contains(limitations, statement) {
			t.Errorf("docs/LIMITATIONS.md does not contain %q", statement)
		}
	}
}

func TestMaintainerNotesMatchRepository(t *testing.T) {
	root := repoRoot(t)
	maintainers := mustReadContractFile(t, root, "MAINTAINERS.md")
	for _, check := range []string{
		"CI / Pull Request Reviewability",
		"CI / Lint and Static Analysis",
		"CI / Test (ubuntu-24.04)",
		"CI / Test (macos-26)",
		"CI / Test (windows-2025)",
		"CodeQL / CodeQL",
		"Dependency Review / Dependency Review",
		"zizmor / Analyze workflows",
	} {
		if strings.Count(maintainers, "`"+check+"`") != 1 {
			t.Errorf("MAINTAINERS.md must contain required check %q exactly once", check)
		}
	}
	codeowners := mustReadContractFile(t, root, ".github/CODEOWNERS")
	if !strings.Contains(codeowners, "* @omkhar") {
		t.Error(".github/CODEOWNERS does not contain the documented wildcard owner rule")
	}
	if !strings.Contains(maintainers, "The wildcard rule in `.github/CODEOWNERS` assigns every repository path") {
		t.Error("MAINTAINERS.md does not describe the wildcard owner rule")
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
	for _, statement := range []string{
		"The `go` directive in `go.mod` is the source for the Go version.",
		"CI installs the latest patch release of that Go minor version.",
	} {
		if !strings.Contains(runtimeDoc, statement) {
			t.Errorf("docs/RUNTIME.md does not contain %q", statement)
		}
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
	if goVersion := goDirectiveVersion(goMod); !goDirectiveVersionPattern.MatchString(goVersion) {
		t.Fatalf("go.mod has an invalid Go version %q", goVersion)
	}

	workflowRoot := filepath.Join(root, ".github", "workflows")
	documentedActions := make(map[string]bool, len(manifest.Actions))
	for _, action := range manifest.Actions {
		documentedActions[action] = true
	}
	var workflowPins []workflowAction
	seenActions := make(map[string]bool)
	seenTools := make(map[string]bool)
	var workflowContent strings.Builder
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
		workflowContent.WriteString(content)
		workflowContent.WriteByte('\n')
		actions, actionErr := workflowActions(content)
		if actionErr != nil {
			t.Errorf("%s: %v", filepath.Base(path), actionErr)
		}
		for _, action := range actions {
			if !documentedActions[action.name] {
				t.Errorf("%s uses undocumented action %s", filepath.Base(path), action.name)
				continue
			}
			seenActions[action.name] = true
		}
		workflowPins = append(workflowPins, actions...)
		if goSetupErr := validateWorkflowGoSetup(content); goSetupErr != nil {
			t.Errorf("%s: %v", filepath.Base(path), goSetupErr)
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
	if err := validateWorkflowActionPins(workflowPins); err != nil {
		t.Error(err)
	}
	for _, action := range manifest.Actions {
		if !seenActions[action] {
			t.Errorf("runtime manifest action %s is not used", action)
		}
	}
	for tool := range manifest.Tools {
		if !seenTools[tool] {
			t.Errorf("runtime manifest tool %s is not installed", tool)
		}
	}
	if err := validateWorkflowToolReferences(manifest.Tools, workflowContent.String()); err != nil {
		t.Error(err)
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
	seenActions := make(map[string]bool, len(manifest.Actions))
	for _, action := range manifest.Actions {
		if action == "" || strings.ContainsAny(action, "@ \t") {
			return fmt.Errorf("runtime manifest action %q must be an action name without a revision", action)
		}
		if seenActions[action] {
			return fmt.Errorf("runtime manifest action %s occurs more than once", action)
		}
		seenActions[action] = true
	}
	digestPattern := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for container, pin := range manifest.Containers {
		if container == "" || !digestPattern.MatchString(pin.Digest) {
			return fmt.Errorf("runtime manifest container %q has an invalid digest", container)
		}
	}
	toolVersionPattern := regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
	for tool, version := range manifest.Tools {
		if tool == "" || !toolVersionPattern.MatchString(version) {
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

type workflowMapping struct {
	number   int
	indent   int
	content  string
	key      string
	value    string
	path     []string
	sequence bool
}

func workflowMappings(content string) ([]workflowMapping, error) {
	lines, err := workflowStructuralLines(content)
	if err != nil {
		return nil, err
	}
	type mappingFrame struct {
		indent int
		key    string
	}
	var stack []mappingFrame
	var mappings []workflowMapping
	for _, line := range lines {
		for len(stack) > 0 && stack[len(stack)-1].indent >= line.indent {
			stack = stack[:len(stack)-1]
		}
		key, value, ok := canonicalWorkflowMapping(line.content)
		if !ok {
			continue
		}
		path := make([]string, 0, len(stack)+1)
		for _, frame := range stack {
			path = append(path, frame.key)
		}
		path = append(path, key)
		mappings = append(mappings, workflowMapping{
			number:   line.number,
			indent:   line.indent,
			content:  line.content,
			key:      key,
			value:    value,
			path:     path,
			sequence: strings.HasPrefix(line.content, "- "),
		})
		trimmedValue := strings.TrimSpace(value)
		if trimmedValue == "" || strings.HasPrefix(trimmedValue, "#") {
			stack = append(stack, mappingFrame{indent: line.indent, key: key})
		}
	}
	return mappings, nil
}

type workflowContainer struct {
	name   string
	digest string
}

func workflowContainers(content string) ([]workflowContainer, error) {
	mappings, err := workflowMappings(normalizeWorkflowContinuations(content))
	if err != nil {
		return nil, err
	}
	pendingFloating := ""
	seen := make(map[string]bool)
	var containers []workflowContainer
	for _, mapping := range mappings {
		if len(mapping.path) != 6 || mapping.path[0] != "jobs" ||
			mapping.path[2] != "strategy" || mapping.path[3] != "matrix" ||
			mapping.path[4] != "include" {
			continue
		}
		if mapping.sequence && pendingFloating != "" {
			return nil, fmt.Errorf("line %d starts a new container entry before its pinned image", mapping.number)
		}
		value := workflowScalar(mapping.value)
		switch mapping.key {
		case "floating_image":
			if pendingFloating != "" {
				return nil, fmt.Errorf("line %d starts a new floating image before its pinned image", mapping.number)
			}
			if value == "" || strings.ContainsAny(value, `\@`) {
				return nil, fmt.Errorf("line %d has an invalid floating image", mapping.number)
			}
			pendingFloating = value
		case "pinned_image":
			if pendingFloating == "" {
				return nil, fmt.Errorf("line %d has a pinned image without a floating image", mapping.number)
			}
			match := regexp.MustCompile(`^([^@]+)@sha256:[ \t]*([0-9a-f]{64})$`).FindStringSubmatch(value)
			if len(match) != 3 {
				return nil, fmt.Errorf("line %d has an invalid pinned image", mapping.number)
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
	mappings, err := workflowMappings(content)
	if err != nil {
		return nil, err
	}
	var actions []workflowAction
	for _, mapping := range mappings {
		isStep := len(mapping.path) == 4 && mapping.path[0] == "jobs" &&
			mapping.path[2] == "steps" && mapping.key == "uses"
		isReusableJob := len(mapping.path) == 3 && mapping.path[0] == "jobs" &&
			mapping.key == "uses"
		if !isStep && !isReusableJob {
			continue
		}
		reference := workflowScalar(mapping.value)
		if strings.Contains(reference, `\`) {
			return nil, fmt.Errorf("line %d uses an escaped action reference", mapping.number)
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

// validateWorkflowActionPins requires a full commit SHA on each external action
// and one SHA for all actions from the same repository.
func validateWorkflowActionPins(actions []workflowAction) error {
	shaPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	repositoryPins := make(map[string]string)
	for _, action := range actions {
		if !shaPattern.MatchString(action.sha) {
			return fmt.Errorf("external action %s uses non-immutable revision %q", action.name, action.sha)
		}
		parts := strings.SplitN(action.name, "/", 3)
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("external action %q has no owner and repository", action.name)
		}
		repository := parts[0] + "/" + parts[1]
		if previous, ok := repositoryPins[repository]; ok && previous != action.sha {
			return fmt.Errorf("actions from %s use two commit SHAs: %s and %s", repository, previous, action.sha)
		}
		repositoryPins[repository] = action.sha
	}
	return nil
}

// validateWorkflowGoSetup requires each actions/setup-go step to install the
// latest patch release of the go.mod minor version. The step must follow the
// canonical resolver step in the same job.
func validateWorkflowGoSetup(content string) error {
	mappings, err := workflowMappings(content)
	if err != nil {
		return err
	}
	type step struct {
		job, id, uses string
		number        int
		with          map[string]string
	}
	var steps []*step
	for _, mapping := range mappings {
		if len(mapping.path) < 4 || mapping.path[0] != "jobs" || mapping.path[2] != "steps" {
			continue
		}
		if len(mapping.path) == 4 && mapping.sequence {
			steps = append(steps, &step{job: mapping.path[1], number: mapping.number, with: make(map[string]string)})
		}
		if len(steps) == 0 || steps[len(steps)-1].job != mapping.path[1] {
			continue
		}
		current := steps[len(steps)-1]
		switch {
		case len(mapping.path) == 4 && mapping.key == "id":
			current.id = workflowScalar(mapping.value)
		case len(mapping.path) == 4 && mapping.key == "uses":
			current.uses = workflowScalar(mapping.value)
		case len(mapping.path) == 5 && mapping.path[3] == "with":
			current.with[mapping.key] = workflowScalar(mapping.value)
		}
	}
	resolvers := 0
	resolvedJobs := make(map[string]bool)
	for _, current := range steps {
		if current.id == "go-version" {
			resolvers++
			resolvedJobs[current.job] = true
		}
		if !strings.HasPrefix(current.uses, "actions/setup-go@") {
			continue
		}
		if !resolvedJobs[current.job] {
			return fmt.Errorf("line %d sets up Go before the Go version resolver step", current.number)
		}
		if _, ok := current.with["go-version-file"]; ok {
			return fmt.Errorf("line %d uses go-version-file, which pins the exact go.mod patch release", current.number)
		}
		if current.with["go-version"] != "${{ steps.go-version.outputs.range }}" || current.with["check-latest"] != "true" {
			return fmt.Errorf("line %d must set go-version to the resolver range and check-latest to true", current.number)
		}
	}
	if got := strings.Count(strings.ReplaceAll(content, "\r\n", "\n"), goVersionResolverStep); got != resolvers {
		return fmt.Errorf("workflow has %d Go version resolver steps, but only %d use the canonical run block", resolvers, got)
	}
	return nil
}

func workflowShellBlocks(content string) ([]string, error) {
	mappings, err := workflowMappings(content)
	if err != nil {
		return nil, err
	}
	rawLines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var blocks []string
	for _, mapping := range mappings {
		if len(mapping.path) != 4 || mapping.path[0] != "jobs" ||
			mapping.path[2] != "steps" || mapping.key != "run" {
			continue
		}
		if !workflowBlockScalarHeader(mapping.content) {
			blocks = append(blocks, workflowScalar(mapping.value))
			continue
		}
		index := mapping.number - 1
		bodyIndent := -1
		var body []string
		for index++; index < len(rawLines); index++ {
			bodyLine := rawLines[index]
			if strings.TrimSpace(bodyLine) == "" {
				body = append(body, "")
				continue
			}
			lineIndent := len(bodyLine) - len(strings.TrimLeft(bodyLine, " "))
			if lineIndent <= mapping.indent {
				break
			}
			if bodyIndent < 0 {
				bodyIndent = lineIndent
			}
			if lineIndent < bodyIndent {
				bodyIndent = lineIndent
			}
			body = append(body, bodyLine)
		}
		if bodyIndent < 0 {
			blocks = append(blocks, "")
			continue
		}
		for bodyIndex, bodyLine := range body {
			if len(bodyLine) >= bodyIndent {
				body[bodyIndex] = bodyLine[bodyIndent:]
			}
		}
		block := strings.Join(body, "\n")
		if strings.HasPrefix(strings.TrimSpace(mapping.value), ">") {
			block = "folded run block\n" + block
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func workflowTools(content string) (map[string]string, error) {
	tools := make(map[string]string)
	record := func(name, version string) error {
		if oldVersion, exists := tools[name]; exists {
			return fmt.Errorf("tool %s occurs more than once at versions %s and %s", name, oldVersion, version)
		}
		tools[name] = version
		return nil
	}
	blocks, err := workflowShellBlocks(content)
	if err != nil {
		return nil, err
	}
	for blockNumber, block := range blocks {
		lines := strings.Split(normalizeWorkflowContinuations(block), "\n")
		for index, line := range lines {
			lines[index] = strings.Join(strings.Fields(line), " ")
		}
		block = strings.Trim(strings.Join(lines, "\n"), "\n")
		installed, canonical := canonicalWorkflowToolBlock(block)
		if canonical {
			for name, version := range installed {
				if err := record(name, version); err != nil {
					return nil, err
				}
			}
			continue
		}
		if workflowBlockReferencesTrackedTool(block) {
			return nil, fmt.Errorf("run block %d references a tracked tool outside a canonical install block", blockNumber+1)
		}
	}
	return tools, nil
}

func canonicalWorkflowToolBlock(block string) (map[string]string, bool) {
	lines := strings.Split(block, "\n")
	if len(lines) == 6 &&
		lines[3] == `echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"` &&
		lines[4] == "sudo apt-get update" &&
		lines[5] == "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends shellcheck" {
		names := []string{
			"github.com/rhysd/actionlint/cmd/actionlint",
			"golang.org/x/vuln/cmd/govulncheck",
			"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		}
		installed := make(map[string]string, len(names))
		for index, name := range names {
			version, ok := workflowGoInstallVersion(lines[index], name)
			if !ok {
				return nil, false
			}
			installed[name] = version
		}
		return installed, true
	}
	if len(lines) == 3 &&
		lines[0] == `python3 -m venv "$RUNNER_TEMP/yamllint-venv"` &&
		lines[2] == `echo "$RUNNER_TEMP/yamllint-venv/bin" >> "$GITHUB_PATH"` {
		const prefix = `"$RUNNER_TEMP/yamllint-venv/bin/pip" install --disable-pip-version-check yamllint==`
		if version, ok := strings.CutPrefix(lines[1], prefix); ok && workflowToolVersionToken(version) {
			return map[string]string{"yamllint": version}, true
		}
	}
	if len(lines) == 2 && lines[1] == `echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"` {
		const name = "github.com/sigstore/cosign/v3/cmd/cosign"
		if version, ok := workflowGoInstallVersion(lines[0], name); ok {
			return map[string]string{name: version}, true
		}
	}
	return nil, false
}

func workflowGoInstallVersion(line, name string) (string, bool) {
	prefix := `GOBIN="$RUNNER_TEMP/bin" go install ` + name + "@"
	version, ok := strings.CutPrefix(line, prefix)
	return version, ok && workflowToolVersionToken(version)
}

func workflowToolVersionToken(version string) bool {
	return version != "" && !strings.ContainsAny(version, " \t\r\n;&|()<>$`\\\"'")
}

func workflowBlockReferencesTrackedTool(block string) bool {
	for _, name := range []string{
		"github.com/rhysd/actionlint/cmd/actionlint",
		"golang.org/x/vuln/cmd/govulncheck",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		"github.com/sigstore/cosign/v3/cmd/cosign",
	} {
		if strings.Contains(block, name) {
			return true
		}
	}
	return strings.Contains(block, "yamllint==")
}

func validateWorkflowToolReferences(tools map[string]string, workflows string) error {
	installed, err := workflowTools(workflows)
	if err != nil {
		return err
	}
	if len(installed) != len(tools) {
		return fmt.Errorf("workflows install %d tracked tools, manifest declares %d", len(installed), len(tools))
	}
	for tool, version := range tools {
		if installed[tool] != version {
			return fmt.Errorf("workflow tool %s must have exactly one reference at version %s", tool, version)
		}
	}
	return nil
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
		name             string
		runsOn           string
		matrixOS         []string
		sawMatrix        bool
		sawMatrixOS      bool
		sawMatrixInclude bool
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
		case len(path) == 5 && path[0] == "jobs" && path[2] == "strategy" &&
			path[3] == "matrix" && key == "include":
			jobs[jobIndexes[path[1]]].sawMatrixInclude = true
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
		if job.sawMatrixInclude {
			return nil, fmt.Errorf("job %s uses matrix.os with an unsupported matrix.include", job.name)
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

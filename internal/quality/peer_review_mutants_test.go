package quality_test

import (
	"fmt"
	"strings"
	"testing"
)

func TestPeerReviewRejectsSeparateExecutableOlderToolInstalls(t *testing.T) {
	const canonical = `GOBIN="$RUNNER_TEMP/bin" go install github.com/sigstore/cosign/v3/cmd/cosign@v3.1.2
echo "$RUNNER_TEMP/bin" >> "$GITHUB_PATH"`
	const oldTool = "github.com/sigstore/cosign/v3/cmd/cosign@v3.0.0"
	mutants := map[string]string{
		"direct indirection": `binary=go
subcommand=install
tool=` + oldTool + `
"$binary" "$subcommand" "$tool"`,
		"eval": `binary=go
subcommand=install
tool=` + oldTool + `
eval '"$binary" "$subcommand" "$tool"'`,
		"nested shell": `(binary=go
subcommand=install
tool=` + oldTool + `
"$binary" "$subcommand" "$tool")`,
		"called function": `install_old_cosign() {
  binary=go
  subcommand=install
  tool=` + oldTool + `
  "$binary" "$subcommand" "$tool"
}
install_old_cosign`,
		"time wrapper": `binary=go
subcommand=install
tool=` + oldTool + `
time "$binary" "$subcommand" "$tool"`,
		"env wrapper": `binary=go
subcommand=install
tool=` + oldTool + `
env GOBIN="$RUNNER_TEMP/bin" "$binary" "$subcommand" "$tool"`,
	}
	tools := map[string]string{
		"github.com/sigstore/cosign/v3/cmd/cosign": "v3.1.2",
	}
	for name, mutant := range mutants {
		t.Run(name, func(t *testing.T) {
			workflow := fmt.Sprintf(`jobs:
  release:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          %s
      - run: |
          %s
`, indentPeerReview(canonical), indentPeerReview(mutant))
			if err := validateWorkflowToolReferences(tools, workflow); err == nil {
				t.Fatal("executable older tool install survived the runtime contract")
			}
		})
	}
}

func TestPeerReviewRejectsMutableToolVersion(t *testing.T) {
	manifest := `{"go":"1.26.5","runners":[],"actions":{},"containers":{},"tools":{"github.com/sigstore/cosign/v3/cmd/cosign":"latest"}}`
	if _, err := decodeRuntimeManifest(strings.NewReader(manifest)); err == nil {
		t.Fatal("mutable tool version survived the runtime manifest contract")
	}
}

func TestPeerReviewIgnoresNonExecutableActionDecoy(t *testing.T) {
	workflow := `env:
  uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: echo test
`
	actions, err := workflowActions(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 0 {
		t.Fatalf("non-executable env.uses produced %d action references", len(actions))
	}
}

func TestPeerReviewIgnoresNonExecutableContainerDecoy(t *testing.T) {
	workflow := `env:
  floating_image: "fedora:44"
  pinned_image: "fedora:44@sha256:6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898"
jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - run: echo test
`
	containers, err := workflowContainers(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if len(containers) != 0 {
		t.Fatalf("non-executable env container pair produced %d container references", len(containers))
	}
}

func indentPeerReview(value string) string {
	return strings.ReplaceAll(value, "\n", "\n          ")
}

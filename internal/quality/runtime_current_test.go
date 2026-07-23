package quality_test

import (
	"strings"
	"testing"
)

func TestAuditedRuntimeVersionsAreCurrent(t *testing.T) {
	root := repoRoot(t)
	manifest, err := decodeRuntimeManifest(strings.NewReader(
		mustReadContractFile(t, root, "docs/runtime-versions.json"),
	))
	if err != nil {
		t.Fatal(err)
	}

	tools := map[string]string{
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint": "v2.12.2",
		"github.com/rhysd/actionlint/cmd/actionlint":             "v1.7.12",
		"github.com/sigstore/cosign/v3/cmd/cosign":               "v3.1.2",
		"golang.org/x/vuln/cmd/govulncheck":                      "v1.6.0",
		"yamllint":                                               "1.38.0",
	}
	if len(manifest.Tools) != len(tools) {
		t.Errorf("runtime manifest has %d tools, want %d", len(manifest.Tools), len(tools))
	}
	for tool, want := range tools {
		if got := manifest.Tools[tool]; got != want {
			t.Errorf("runtime manifest tool %s = %q, want %q", tool, got, want)
		}
	}

	containers := map[string]string{
		"debian:stable-slim":   "328d16499860ae6cb9b345e2e4cebca08c2a36e4f7278482c7bd1f39d71e5bfd",
		"debian:unstable-slim": "153e7023d3501a4a980f54fd3e8560c029109bb1a3475fcc5047fc948da553ab",
		"fedora:44":            "6c75d5bf57cb0fa5aa4b92c6a83c86c791644496d9ac230de7711f5b8ec3b898",
		"fedora:rawhide":       "0c1f63ed8fb818fad16cf6ae091598c410a21d2e1a9adf183beb93189299bfba",
		"ubuntu:devel":         "694b773ee7e0d0b55ca74c095ac3309055589e7cbfaf3100f0b226c38c6936fa",
		"ubuntu:rolling":       "3131b4cc82a783df6c9df078f86e01819a13594b865c2cad47bd1bca2b7063bb",
	}
	if len(manifest.Containers) != len(containers) {
		t.Errorf("runtime manifest has %d containers, want %d", len(manifest.Containers), len(containers))
	}
	for container, want := range containers {
		if got := manifest.Containers[container].Digest; got != want {
			t.Errorf("runtime manifest container %s = %q, want %q", container, got, want)
		}
	}
}

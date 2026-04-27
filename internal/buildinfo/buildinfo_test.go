package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestVersionStringUsesInjectedVersion(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		Version = originalVersion
	})
	Version = "1.2.3"

	if got, want := VersionString(), "1.2.3"; got != want {
		t.Fatalf("VersionString() = %q, want %q", got, want)
	}
	if got, want := CommandLine(), "dns-update 1.2.3"; got != want {
		t.Fatalf("CommandLine() = %q, want %q", got, want)
	}
	if got, want := UserAgent(), "dns-update/1.2.3"; got != want {
		t.Fatalf("UserAgent() = %q, want %q", got, want)
	}
}

func TestVersionStringUsesBuildInfo(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		Version = originalVersion
	})
	Version = ""

	got := versionString(func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{
				Version: "v1.2.3",
			},
		}, true
	})
	if want := "v1.2.3"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionStringUsesRuntimeBuildInfo(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() {
		Version = originalVersion
	})
	Version = ""

	if got := VersionString(); got == "" {
		t.Fatal("VersionString() = empty, want runtime build info version")
	}
}

func TestVersionStringFallback(t *testing.T) {
	got := versionString(func() (*debug.BuildInfo, bool) {
		return nil, false
	})
	if want := "(devel)"; got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
	if got, want := userAgent(got), "dns-update/devel"; got != want {
		t.Fatalf("userAgent() = %q, want %q", got, want)
	}
}

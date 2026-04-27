package buildinfo

import "runtime/debug"

const Product = "dns-update"

// Version is set by release builds with -ldflags "-X dns-update/internal/buildinfo.Version=<version>".
var Version string

func CommandLine() string {
	return Product + " " + VersionString()
}

func UserAgent() string {
	return userAgent(VersionString())
}

func VersionString() string {
	if Version != "" {
		return Version
	}
	return versionString(debug.ReadBuildInfo)
}

func versionString(read func() (*debug.BuildInfo, bool)) string {
	if info, ok := read(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func userAgent(version string) string {
	if version == "(devel)" {
		version = "devel"
	}
	return Product + "/" + version
}

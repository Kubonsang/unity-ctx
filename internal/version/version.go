// Package version holds the build version string, overridable at link time via
// -ldflags "-X github.com/Kubonsang/unity-ctx/internal/version.Version=<tag>".
package version

import "runtime/debug"

// Version is the unity-ctx build version. Defaults to "dev" for plain source
// builds; release builds inject the git tag via -ldflags, and `go install
// <module>@<tag>` recovers it from the module build info (see init).
var Version = "dev"

func init() {
	// If -ldflags injected a version, trust it. Otherwise fall back to the
	// module version Go embeds for `go install module@version` builds.
	if Version != "dev" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		Version = v
	}
}

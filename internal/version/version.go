// Package version holds the build version string, overridable at link time via
// -ldflags "-X github.com/Kubonsang/unity-ctx/internal/version.Version=<tag>".
package version

// Version is the unity-ctx build version. Defaults to "dev" for source builds;
// release builds inject the git tag.
var Version = "dev"

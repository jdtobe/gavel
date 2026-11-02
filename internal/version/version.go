// Package version holds the build version reported by the gavel binary.
package version

// Version is the build version. Release builds override it with
// -ldflags "-X github.com/jdtobe/gavel/internal/version.Version=<version>".
var Version = "dev"

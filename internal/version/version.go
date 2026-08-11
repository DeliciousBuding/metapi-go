// Package version exposes the build-time version of the metapi binary.
package version

// Version is injected at build time via
// -ldflags "-X github.com/deliciousbuding/metapi-go/internal/version.Version=vX.Y.Z".
// Local builds without injection report "dev".
var Version = "dev"

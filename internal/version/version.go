// Package version exposes the build-time version of the metapi binary.
package version

// Version is injected at build time via
// -ldflags "-X github.com/deliciousbuding/metapi-go/internal/version.Version=vX.Y.Z".
// Local builds without injection report "dev".
var Version = "dev"

// Commit is the git commit SHA the binary was built from, injected via
// -ldflags "-X github.com/deliciousbuding/metapi-go/internal/version.Commit=<sha>".
// Stays empty for builds without injection (local `go build`) — callers must
// surface "unknown" rather than substituting a placeholder SHA.
var Commit = ""

// BuildTime is the UTC RFC3339 timestamp of the build, injected via
// -ldflags "-X github.com/deliciousbuding/metapi-go/internal/version.BuildTime=<ts>".
// Stays empty for builds without injection (local `go build`) — callers must
// surface "unknown" rather than substituting the process start time.
var BuildTime = ""

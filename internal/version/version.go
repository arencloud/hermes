package version

// Package version holds build-time injected metadata about the application.
// Values are set via -ldflags in Makefile and Docker builds.

var (
    // Version is the application version (e.g., 1.2.3). Default: dev.
    Version = "dev"
    // Commit is the git commit SHA used for this build.
    Commit = ""
    // BuildDate is the UTC build timestamp in RFC3339 format.
    BuildDate = ""
)

package version

// Version holds the application version. It is overridden at build time via:
//   -ldflags "-X github.com/arencloud/hermes/internal/version.Version=vX.Y.Z"
// Default is "dev" when not set (e.g., local builds without tags).
var Version = "dev"

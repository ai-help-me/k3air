package version

var (
	// Version is the version string, set via ldflags at build time
	// Format: "v1.0.0", "dev-20250202", etc.
	Version = "unknown"

	// BuildTime is the build timestamp, set via ldflags at build time
	BuildTime = "unknown"

	// GitCommit is the git commit hash, set via ldflags at build time
	GitCommit = "unknown"
)

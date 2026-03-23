package config

// Version and BuildTime are set at link time via -ldflags.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

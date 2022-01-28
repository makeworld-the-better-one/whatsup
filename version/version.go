package version

import "fmt"

// Version info. This is set by the Makefile
var (
	version string
	commit  string
	builtBy string
)

var VersionInfo = fmt.Sprintf("Version: %s\nCommit: %s\nBuilt by: %s\n", version, commit, builtBy)

package version

import (
	"fmt"
	"runtime"
)

const Version = "1.0.0"

var (
	Commit    = "dev"
	BuildDate = "unknown"
)

func Full() string {
	return fmt.Sprintf("GogoBee v%s (%s, %s, %s)", Version, Commit, BuildDate, runtime.Version())
}

func Short() string {
	return fmt.Sprintf("v%s-%s", Version, shortCommit())
}

func shortCommit() string {
	if len(Commit) > 7 {
		return Commit[:7]
	}
	return Commit
}

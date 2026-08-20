package updatecheck

import (
	"os"
	"strings"
)

// buildVersion and buildCommit are stamped at link time:
// -ldflags "-X github.com/fjaeckel/ninerlog-api/internal/updatecheck.buildVersion=v1.2.3
//
//	-X github.com/fjaeckel/ninerlog-api/internal/updatecheck.buildCommit=<sha>".
var (
	buildVersion string
	buildCommit  string
)

// DevVersion is the version reported by a binary that was neither stamped at
// build time nor given APP_VERSION.
const DevVersion = "dev"

// RunningVersion returns the version of this binary: the link-time stamp, then
// APP_VERSION, then DevVersion.
func RunningVersion() string {
	if v := strings.TrimSpace(buildVersion); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("APP_VERSION")); v != "" {
		return v
	}
	return DevVersion
}

// RunningCommit returns the commit this binary was built from: the link-time
// stamp, then APP_COMMIT. Empty when neither is set or the value is not a
// commit SHA.
func RunningCommit() string {
	for _, candidate := range []string{buildCommit, os.Getenv("APP_COMMIT")} {
		if sha, ok := normalizeCommit(candidate); ok {
			return sha
		}
	}
	return ""
}

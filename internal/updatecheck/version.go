package updatecheck

import (
	"os"
	"strings"
)

// buildVersion is stamped at link time:
// -ldflags "-X github.com/fjaeckel/ninerlog-api/internal/updatecheck.buildVersion=v1.2.3".
var buildVersion string

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

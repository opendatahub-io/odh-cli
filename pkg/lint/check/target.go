package check

import (
	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// Target holds all context needed for executing diagnostic checks, including cluster version and optional resource.
type Target struct {
	// Client provides read-only access to Kubernetes API for querying resources.
	// Uses the Reader interface to enforce that lint checks cannot perform write operations.
	Client client.Reader

	// CurrentVersion contains the current/source cluster version as parsed semver
	// For lint mode: same as TargetVersion
	// For upgrade mode: the version being upgraded FROM
	// Nil if no current version available
	CurrentVersion *semver.Version

	// TargetVersion contains the target version as parsed semver
	// For lint mode: the detected cluster version
	// For upgrade mode: the version being upgraded TO
	// Nil if no target version available
	TargetVersion *semver.Version

	// Resource is the specific resource being validated (optional)
	// Only set for workload checks that operate on discovered CRs
	// Nil for component and service checks
	Resource *unstructured.Unstructured

	// IO provides access to input/output streams for logging (optional)
	// Used by checks to log warnings (e.g., permission errors) when verbose mode is enabled
	// If nil, checks should skip logging
	IO iostreams.Interface

	// Debug enables detailed diagnostic logging for troubleshooting
	// When true, checks should emit internal processing logs for troubleshooting
	// When false, only user-facing summary information should be logged via IO
	Debug bool
}

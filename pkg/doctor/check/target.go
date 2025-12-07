package check

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// CheckTarget represents the target environment for check execution.
type CheckTarget struct {
	// Client provides access to Kubernetes API for querying resources
	Client *client.Client

	// CurrentVersion contains the current/source cluster version information
	// For lint mode: same as Version
	// For upgrade mode: the version being upgraded FROM
	CurrentVersion *version.ClusterVersion

	// Version contains the target version information
	// For lint mode: the detected cluster version
	// For upgrade mode: the version being upgraded TO
	Version *version.ClusterVersion

	// Resource is the specific resource being validated (optional)
	// Only set for workload checks that operate on discovered CRs
	// Nil for component and service checks
	Resource *unstructured.Unstructured
}

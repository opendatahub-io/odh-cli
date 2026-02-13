package pipeline

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
)

// WorkloadItem represents a workload instance to back up.
type WorkloadItem struct {
	GVR      schema.GroupVersionResource
	Instance *unstructured.Unstructured
}

// WorkloadWithDeps represents a workload with resolved dependencies.
type WorkloadWithDeps struct {
	GVR          schema.GroupVersionResource
	Instance     *unstructured.Unstructured
	Dependencies []dependencies.Dependency
}

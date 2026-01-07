package resources

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ResourceType defines a Kubernetes resource with its GroupVersionKind and GroupVersionResource.
type ResourceType struct {
	Group    string
	Version  string
	Kind     string
	Resource string
}

// GVK returns the GroupVersionKind for this resource.
func (r ResourceType) GVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   r.Group,
		Version: r.Version,
		Kind:    r.Kind,
	}
}

// GVR returns the GroupVersionResource for this resource.
func (r ResourceType) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    r.Group,
		Version:  r.Version,
		Resource: r.Resource,
	}
}

// ListKind returns the list kind name for this resource (Kind + "List").
func (r ResourceType) ListKind() string {
	return r.Kind + "List"
}

// APIVersion returns the apiVersion string (group/version or just version for core resources).
func (r ResourceType) APIVersion() string {
	if r.Group == "" {
		return r.Version
	}

	return r.Group + "/" + r.Version
}

// Centralized resource type definitions (Principle VIII)
// All GVK/GVR references MUST use these definitions, not inline construction
//
//nolint:gochecknoglobals // Required by Constitution Principle VIII - centralized GVK/GVR definitions
var (
	// DataScienceCluster is the OpenShift AI DataScienceCluster resource.
	DataScienceCluster = ResourceType{
		Group:    "datasciencecluster.opendatahub.io",
		Version:  "v1",
		Kind:     "DataScienceCluster",
		Resource: "datascienceclusters",
	}

	DSCInitialization = ResourceType{
		Group:    "dscinitialization.opendatahub.io",
		Version:  "v1",
		Kind:     "DSCInitialization",
		Resource: "dscinitializations",
	}

	// Deployment is the Kubernetes Deployment resource.
	Deployment = ResourceType{
		Group:    "apps",
		Version:  "v1",
		Kind:     "Deployment",
		Resource: "deployments",
	}

	Pod = ResourceType{
		Group:    "",
		Version:  "v1",
		Kind:     "Pod",
		Resource: "pods",
	}

	Service = ResourceType{
		Group:    "",
		Version:  "v1",
		Kind:     "Service",
		Resource: "services",
	}

	ConfigMap = ResourceType{
		Group:    "",
		Version:  "v1",
		Kind:     "ConfigMap",
		Resource: "configmaps",
	}

	Secret = ResourceType{
		Group:    "",
		Version:  "v1",
		Kind:     "Secret",
		Resource: "secrets",
	}

	// CustomResourceDefinition is the Kubernetes CustomResourceDefinition resource.
	CustomResourceDefinition = ResourceType{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Kind:     "CustomResourceDefinition",
		Resource: "customresourcedefinitions",
	}

	// ClusterServiceVersion is the OLM ClusterServiceVersion resource for version detection.
	ClusterServiceVersion = ResourceType{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Kind:     "ClusterServiceVersion",
		Resource: "clusterserviceversions",
	}

	Subscription = ResourceType{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Kind:     "Subscription",
		Resource: "subscriptions",
	}

	InstallPlan = ResourceType{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Kind:     "InstallPlan",
		Resource: "installplans",
	}

	// ClusterQueue is the Kueue ClusterQueue resource.
	ClusterQueue = ResourceType{
		Group:    "kueue.x-k8s.io",
		Version:  "v1beta1",
		Kind:     "ClusterQueue",
		Resource: "clusterqueues",
	}

	// LocalQueue is the Kueue LocalQueue resource.
	LocalQueue = ResourceType{
		Group:    "kueue.x-k8s.io",
		Version:  "v1beta1",
		Kind:     "LocalQueue",
		Resource: "localqueues",
	}

	// InferenceService is the KServe InferenceService resource.
	InferenceService = ResourceType{
		Group:    "serving.kserve.io",
		Version:  "v1beta1",
		Kind:     "InferenceService",
		Resource: "inferenceservices",
	}

	// ServingRuntime is the KServe ServingRuntime resource.
	ServingRuntime = ResourceType{
		Group:    "serving.kserve.io",
		Version:  "v1alpha1",
		Kind:     "ServingRuntime",
		Resource: "servingruntimes",
	}

	// RayCluster is the Ray RayCluster resource.
	RayCluster = ResourceType{
		Group:    "ray.io",
		Version:  "v1",
		Kind:     "RayCluster",
		Resource: "rayclusters",
	}
)

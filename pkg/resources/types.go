package resources

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

// TypeMeta returns a metav1.TypeMeta for this resource type.
func (r ResourceType) TypeMeta() metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: r.APIVersion(),
		Kind:       r.Kind,
	}
}

// Unstructured returns a new unstructured.Unstructured with the GVK set.
func (r ResourceType) Unstructured() unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetGroupVersionKind(r.GVK())

	return obj
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

	// DataSciencePipelinesApplicationV1 is the DSP DataSciencePipelinesApplication resource (v1).
	DataSciencePipelinesApplicationV1 = ResourceType{
		Group:    "datasciencepipelinesapplications.opendatahub.io",
		Version:  "v1",
		Kind:     "DataSciencePipelinesApplication",
		Resource: "datasciencepipelinesapplications",
	}

	// DataSciencePipelinesApplicationV1Alpha1 is the DSP DataSciencePipelinesApplication resource (v1alpha1).
	DataSciencePipelinesApplicationV1Alpha1 = ResourceType{
		Group:    "datasciencepipelinesapplications.opendatahub.io",
		Version:  "v1alpha1",
		Kind:     "DataSciencePipelinesApplication",
		Resource: "datasciencepipelinesapplications",
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

	PersistentVolumeClaim = ResourceType{
		Group:    "",
		Version:  "v1",
		Kind:     "PersistentVolumeClaim",
		Resource: "persistentvolumeclaims",
	}

	// Notebook is the Kubeflow Notebook resource.
	Notebook = ResourceType{
		Group:    "kubeflow.org",
		Version:  "v1",
		Kind:     "Notebook",
		Resource: "notebooks",
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

	// PyTorchJob is the Kubeflow Training PyTorchJob resource.
	PyTorchJob = ResourceType{
		Group:    "kubeflow.org",
		Version:  "v1",
		Kind:     "PyTorchJob",
		Resource: "pytorchjobs",
	}

	// GuardrailsOrchestrator is the TrustyAI GuardrailsOrchestrator resource.
	GuardrailsOrchestrator = ResourceType{
		Group:    "trustyai.opendatahub.io",
		Version:  "v1alpha1",
		Kind:     "GuardrailsOrchestrator",
		Resource: "guardrailsorchestrators",
	}

	// ClusterVersion is the OpenShift cluster version resource.
	ClusterVersion = ResourceType{
		Group:    "config.openshift.io",
		Version:  "v1",
		Kind:     "ClusterVersion",
		Resource: "clusterversions",
	}

	// AcceleratorProfile is the OpenShift AI AcceleratorProfile resource.
	AcceleratorProfile = ResourceType{
		Group:    "dashboard.opendatahub.io",
		Version:  "v1",
		Kind:     "AcceleratorProfile",
		Resource: "acceleratorprofiles",
	}

	// HardwareProfile is the OpenShift AI HardwareProfile resource in the old API group.
	// During upgrade to 3.x, these are auto-migrated to infrastructure.opendatahub.io.
	HardwareProfile = ResourceType{
		Group:    "dashboard.opendatahub.io",
		Version:  "v1alpha1",
		Kind:     "HardwareProfile",
		Resource: "hardwareprofiles",
	}
)

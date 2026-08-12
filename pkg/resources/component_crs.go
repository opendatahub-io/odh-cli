package resources

import "sort"

const (
	componentCRGroup = "components.platform.opendatahub.io"

	// ComponentLabelKey is the label key used to identify ODH components.
	ComponentLabelKey = "app.kubernetes.io/part-of"
)

// ComponentCRResourceTypes maps DSC component names to their corresponding
// component CR resource types from the components.platform.opendatahub.io API group.
// These CRs are v1alpha1 and may not exist on older ODH versions.
//
//nolint:gochecknoglobals // Component CR mapping is static configuration
var ComponentCRResourceTypes = map[string]ResourceType{
	// DSC v2 component names
	"aigateway":          newComponentCR("aigateways", "AIGateway"),
	"aipipelines":        newComponentCR("datasciencepipelines", "DataSciencePipelines"),
	"dashboard":          newComponentCR("dashboards", "Dashboard"),
	"feastoperator":      newComponentCR("feastoperators", "FeastOperator"),
	"kserve":             newComponentCR("kserves", "Kserve"),
	"llamastackoperator": newComponentCR("llamastackoperators", "LlamaStackOperator"),
	"mlflowoperator":     newComponentCR("mlflowoperators", "MLflowOperator"),
	"modelregistry":      newComponentCR("modelregistries", "ModelRegistry"),
	"ray":                newComponentCR("rays", "Ray"),
	"sparkoperator":      newComponentCR("sparkoperators", "SparkOperator"),
	"trainer":            newComponentCR("trainers", "Trainer"),
	"trainingoperator":   newComponentCR("trainingoperators", "TrainingOperator"),
	"trustyai":           newComponentCR("trustyais", "TrustyAI"),
	"workbenches":        newComponentCR("workbenches", "Workbenches"),
}

// ComponentNames returns a sorted list of all valid component names.
func ComponentNames() []string {
	names := make([]string, 0, len(ComponentCRResourceTypes))
	for name := range ComponentCRResourceTypes {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func newComponentCR(resource, kind string) ResourceType {
	return ResourceType{
		Group:    componentCRGroup,
		Version:  "v1alpha1",
		Kind:     kind,
		Resource: resource,
	}
}

// GetComponentCR returns the ResourceType for a component CR by name.
// Returns nil if the component is not found.
func GetComponentCR(name string) *ResourceType {
	rt, ok := ComponentCRResourceTypes[name]
	if !ok {
		return nil
	}

	return &rt
}

// componentLabelOverrides maps component names to their actual label values
// when they differ from the component name.
//
// NOTE: When adding new components to ComponentCRResourceTypes, verify the actual
// app.kubernetes.io/part-of label value used by that component's resources.
// If it differs from the CLI name, add an override here to avoid empty --component results.
//
//nolint:gochecknoglobals // Static label mapping configuration
var componentLabelOverrides = map[string]string{
	"aipipelines":   "data-science-pipelines-operator",
	"modelregistry": "model-registry-operator",
}

// GetComponentLabelValue returns the label value used for a component.
// Some components use different label values than their CLI names.
func GetComponentLabelValue(component string) string {
	if override, ok := componentLabelOverrides[component]; ok {
		return override
	}

	return component
}

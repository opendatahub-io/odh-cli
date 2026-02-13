package guardrails

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	annotationIssues = "guardrails.opendatahub.io/issues"
)

// crConfig holds the ConfigMap names extracted from a GuardrailsOrchestrator spec.
type crConfig struct {
	orchestratorConfigName string
	gatewayConfigName      string
}

// crResult holds the aggregated validation result for a single CR,
// including ConfigMap names and issue annotations.
type crResult struct {
	orchCMName    string
	gatewayCMName string
	annotations   map[string]string
}

// validateCR validates a single GuardrailsOrchestrator CR and returns the
// aggregated result including spec checks, ConfigMap checks, and annotations.
func (c *ImpactedWorkloadsCheck) validateCR(
	ctx context.Context,
	reader client.Reader,
	obj *unstructured.Unstructured,
) crResult {
	var cr crResult

	cr.annotations = map[string]string{}
	sr := c.validateCRSpec(obj)

	cr.orchCMName = sr.config.orchestratorConfigName
	cr.gatewayCMName = sr.config.gatewayConfigName

	if sr.orchConfigFail {
		cr.annotations[annotationOrchestratorConfig] = "not set"
	}

	// Combined gateway: two sub-fields merged into one annotation with detailed description.
	if sr.gatewayFail || sr.gatewayConfigFail {
		var details []string
		if sr.gatewayFail {
			details = append(details, "enableGuardrailsGateway not enabled")
		}

		if sr.gatewayConfigFail {
			details = append(details, "guardrailsGatewayConfig not set")
		}

		cr.annotations[annotationGatewayConfig] = strings.Join(details, "; ")
	}

	if sr.detectorsFail {
		cr.annotations[annotationBuiltinDetectors] = "not enabled"
	}

	if sr.config.orchestratorConfigName != "" {
		orchIssues := c.validateOrchestratorConfigMap(ctx, reader, obj.GetNamespace(), sr.config.orchestratorConfigName)
		if len(orchIssues) > 0 {
			cr.annotations[annotationOrchestratorCM] = strings.Join(orchIssues, "; ")
		}
	}

	if sr.config.gatewayConfigName != "" {
		gatewayIssues := c.validateGatewayConfigMap(ctx, reader, obj.GetNamespace(), sr.config.gatewayConfigName)
		if len(gatewayIssues) > 0 {
			cr.annotations[annotationGatewayCM] = strings.Join(gatewayIssues, "; ")
		}
	}

	// Clear annotations map if no issues found to avoid empty-map impacted objects.
	if len(cr.annotations) == 0 {
		cr.annotations = nil
	}

	return cr
}

// specResult holds per-field validation results from CR spec validation.
type specResult struct {
	config            crConfig
	orchConfigFail    bool
	gatewayFail       bool
	detectorsFail     bool
	gatewayConfigFail bool
}

// validateCRSpec validates the spec fields of a GuardrailsOrchestrator CR
// and returns per-field validation results.
func (c *ImpactedWorkloadsCheck) validateCRSpec(obj *unstructured.Unstructured) specResult {
	var r specResult

	r.config.orchestratorConfigName, r.orchConfigFail = c.checkStringFieldMissing(obj, ".spec.orchestratorConfig")
	r.gatewayFail = c.checkBoolNotTrue(obj, ".spec.enableGuardrailsGateway")
	r.detectorsFail = c.checkBoolNotTrue(obj, ".spec.enableBuiltInDetectors")
	r.config.gatewayConfigName, r.gatewayConfigFail = c.checkStringFieldMissing(obj, ".spec.guardrailsGatewayConfig")

	return r
}

// checkStringFieldMissing returns the field value and whether it's missing or empty.
func (c *ImpactedWorkloadsCheck) checkStringFieldMissing(
	obj *unstructured.Unstructured,
	query string,
) (string, bool) {
	val, err := jq.Query[string](obj, query)
	if err != nil || val == "" {
		return "", true
	}

	return val, false
}

// checkBoolNotTrue returns true if the field is missing or not set to true.
func (c *ImpactedWorkloadsCheck) checkBoolNotTrue(
	obj *unstructured.Unstructured,
	query string,
) bool {
	val, err := jq.Query[bool](obj, query)

	return err != nil || !val
}

// validateOrchestratorConfigMap validates the orchestrator ConfigMap's config.yaml content.
// Returns a list of issues found.
func (c *ImpactedWorkloadsCheck) validateOrchestratorConfigMap(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	name string,
) []string {
	cm, err := reader.GetResource(ctx, resources.ConfigMap, name, client.InNamespace(namespace))
	if err != nil {
		return []string{"orchestrator ConfigMap not found"}
	}

	if cm == nil {
		return []string{"orchestrator ConfigMap not found"}
	}

	// Extract config.yaml from the ConfigMap data.
	configYAML, err := jq.Query[string](cm, ".data[\"config.yaml\"]")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return []string{"orchestrator ConfigMap missing config.yaml"}
		}

		return []string{fmt.Sprintf("failed to query config.yaml from orchestrator ConfigMap: %v", err)}
	}

	if configYAML == "" {
		return []string{"orchestrator ConfigMap has empty config.yaml"}
	}

	// Parse the YAML content.
	var configData map[string]any
	if err := yaml.Unmarshal([]byte(configYAML), &configData); err != nil {
		return []string{"orchestrator ConfigMap has invalid config.yaml"}
	}

	return c.validateOrchestratorConfigData(configData)
}

// validateOrchestratorConfigData checks the parsed config.yaml content for required fields.
// Returns category-level issues rather than individual field names.
func (c *ImpactedWorkloadsCheck) validateOrchestratorConfigData(configData map[string]any) []string {
	var issues []string

	// Check chat_generation.service category (hostname + port).
	chatGenIssue := false

	hostname, err := jq.Query[string](configData, ".chat_generation.service.hostname")
	if err != nil || hostname == "" {
		chatGenIssue = true
	}

	port, err := jq.Query[any](configData, ".chat_generation.service.port")
	if err != nil || fmt.Sprintf("%v", port) == "" {
		chatGenIssue = true
	}

	if chatGenIssue {
		issues = append(issues, "chat_generation.service misconfiguration")
	}

	// Check detectors category.
	detectors, err := jq.Query[any](configData, ".detectors")
	if err != nil {
		issues = append(issues, "detectors misconfiguration")
	} else if detectorsList, ok := detectors.([]any); ok && len(detectorsList) == 0 {
		issues = append(issues, "detectors misconfiguration")
	}

	return issues
}

// validateGatewayConfigMap validates the gateway ConfigMap exists.
// Returns a list of issues found.
func (c *ImpactedWorkloadsCheck) validateGatewayConfigMap(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	name string,
) []string {
	cm, err := reader.GetResource(ctx, resources.ConfigMap, name, client.InNamespace(namespace))
	if err != nil {
		return []string{"gateway ConfigMap not found"}
	}

	if cm == nil {
		return []string{"gateway ConfigMap not found"}
	}

	return nil
}

// newConfigurationCondition creates a single consolidated condition for all
// GuardrailsOrchestrator configuration validation.
func (c *ImpactedWorkloadsCheck) newConfigurationCondition(
	total int,
	impacted int,
) result.Condition {
	if total == 0 {
		return check.NewCondition(
			ConditionTypeConfigurationValid,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No GuardrailsOrchestrators found"),
		)
	}

	if impacted == 0 {
		return check.NewCondition(
			ConditionTypeConfigurationValid,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("All %d GuardrailsOrchestrator(s) configured correctly", total),
		)
	}

	return check.NewCondition(
		ConditionTypeConfigurationValid,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonConfigurationInvalid),
		check.WithMessage("Found %d misconfigured GuardrailsOrchestrator(s)", impacted),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)
}

// appendImpactedObject adds a GuardrailsOrchestrator to the impacted objects list
// with annotations describing the specific issues found on this CR.
func (c *ImpactedWorkloadsCheck) appendImpactedObject(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	annotations map[string]string,
) {
	dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
		TypeMeta: resources.GuardrailsOrchestrator.TypeMeta(),
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   obj.GetNamespace(),
			Name:        obj.GetName(),
			Annotations: annotations,
		},
	})
}

// appendImpactedConfigMaps adds ConfigMap references to the impacted objects list
// when they have issues detected during validation.
func (c *ImpactedWorkloadsCheck) appendImpactedConfigMaps(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	cr crResult,
) {
	if cr.annotations == nil {
		return
	}

	ns := obj.GetNamespace()

	// Orchestrator ConfigMap with issues.
	if issues, ok := cr.annotations[annotationOrchestratorCM]; ok && cr.orchCMName != "" {
		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.ConfigMap.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      cr.orchCMName,
				Annotations: map[string]string{
					annotationIssues: issues,
				},
			},
		})
	}

	// Gateway ConfigMap with issues.
	if issues, ok := cr.annotations[annotationGatewayCM]; ok && cr.gatewayCMName != "" {
		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.ConfigMap.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      cr.gatewayCMName,
				Annotations: map[string]string{
					annotationIssues: issues,
				},
			},
		})
	}
}

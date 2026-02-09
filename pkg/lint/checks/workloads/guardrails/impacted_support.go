package guardrails

import (
	"context"
	"errors"
	"fmt"

	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// crConfig holds the ConfigMap names extracted from a GuardrailsOrchestrator spec.
type crConfig struct {
	orchestratorConfigName string
	gatewayConfigName      string
}

// validateCRSpec validates the spec fields of a GuardrailsOrchestrator CR.
// Returns the extracted config and a list of issues found.
func validateCRSpec(obj *unstructured.Unstructured) (crConfig, []string) {
	var cfg crConfig

	var issues []string

	cfg.orchestratorConfigName, issues = requireStringField(obj, ".spec.orchestratorConfig", "orchestratorConfig", issues)
	issues = requireMinReplicas(obj, issues)
	issues = requireBoolTrue(obj, ".spec.enableGuardrailsGateway", "enableGuardrailsGateway", issues)
	issues = requireBoolTrue(obj, ".spec.enableBuiltInDetectors", "enableBuiltInDetectors", issues)
	cfg.gatewayConfigName, issues = requireStringField(obj, ".spec.guardrailsGatewayConfig", "guardrailsGatewayConfig", issues)

	return cfg, issues
}

// requireStringField validates a non-empty string field and returns its value.
func requireStringField(
	obj *unstructured.Unstructured,
	query string,
	fieldName string,
	issues []string,
) (string, []string) {
	val, err := jq.Query[string](obj, query)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return "", append(issues, fieldName+" is not set")
		}

		return "", append(issues, "failed to query "+fieldName+": "+err.Error())
	}

	if val == "" {
		return "", append(issues, fieldName+" is empty")
	}

	return val, issues
}

// requireBoolTrue validates that a boolean field is set to true.
func requireBoolTrue(
	obj *unstructured.Unstructured,
	query string,
	fieldName string,
	issues []string,
) []string {
	val, err := jq.Query[bool](obj, query)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return append(issues, fieldName+" is not set")
		}

		return append(issues, "failed to query "+fieldName+": "+err.Error())
	}

	if !val {
		return append(issues, fieldName+" is false, expected true")
	}

	return issues
}

// requireMinReplicas validates that .spec.replicas >= 1.
func requireMinReplicas(obj *unstructured.Unstructured, issues []string) []string {
	replicas, err := jq.Query[float64](obj, ".spec.replicas")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return append(issues, "replicas is not set")
		}

		return append(issues, fmt.Sprintf("failed to query replicas: %v", err))
	}

	if replicas < 1 {
		return append(issues, fmt.Sprintf("replicas is %d, expected >= 1", int(replicas)))
	}

	return issues
}

// validateOrchestratorConfigMap validates the orchestrator ConfigMap's config.yaml content.
// Returns a list of issues found.
func validateOrchestratorConfigMap(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	name string,
) []string {
	cm, err := reader.GetResource(ctx, resources.ConfigMap, name, client.InNamespace(namespace))
	if err != nil {
		return []string{fmt.Sprintf("failed to get orchestrator ConfigMap %q: %v", name, err)}
	}

	if cm == nil {
		return []string{fmt.Sprintf("orchestrator ConfigMap %q not found", name)}
	}

	// Extract config.yaml from the ConfigMap data
	configYAML, err := jq.Query[string](cm, ".data[\"config.yaml\"]")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return []string{fmt.Sprintf("orchestrator ConfigMap %q missing config.yaml key", name)}
		}

		return []string{fmt.Sprintf("failed to query config.yaml from ConfigMap %q: %v", name, err)}
	}

	if configYAML == "" {
		return []string{fmt.Sprintf("orchestrator ConfigMap %q has empty config.yaml", name)}
	}

	// Parse the YAML content
	var configData map[string]any
	if err := yaml.Unmarshal([]byte(configYAML), &configData); err != nil {
		return []string{fmt.Sprintf("orchestrator ConfigMap %q has invalid config.yaml YAML: %v", name, err)}
	}

	return validateOrchestratorConfigData(name, configData)
}

// validateOrchestratorConfigData checks the parsed config.yaml content for required fields.
func validateOrchestratorConfigData(name string, configData map[string]any) []string {
	var issues []string

	// Check chat_generation.service.hostname
	hostname, err := jq.Query[string](configData, ".chat_generation.service.hostname")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml missing chat_generation.service.hostname", name))
		} else {
			issues = append(issues, fmt.Sprintf("ConfigMap %q failed to query hostname: %v", name, err))
		}
	} else if hostname == "" {
		issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml has empty chat_generation.service.hostname", name))
	}

	// Check chat_generation.service.port
	port, err := jq.Query[any](configData, ".chat_generation.service.port")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml missing chat_generation.service.port", name))
		} else {
			issues = append(issues, fmt.Sprintf("ConfigMap %q failed to query port: %v", name, err))
		}
	} else if fmt.Sprintf("%v", port) == "" {
		issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml has empty chat_generation.service.port", name))
	}

	// Check detectors list is non-empty
	detectors, err := jq.Query[any](configData, ".detectors")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml missing detectors", name))
		} else {
			issues = append(issues, fmt.Sprintf("ConfigMap %q failed to query detectors: %v", name, err))
		}
	} else if detectorsList, ok := detectors.([]any); ok && len(detectorsList) == 0 {
		issues = append(issues, fmt.Sprintf("ConfigMap %q config.yaml has empty detectors list", name))
	}

	return issues
}

// validateGatewayConfigMap validates the gateway ConfigMap exists.
// Returns a list of issues found.
func validateGatewayConfigMap(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	name string,
) []string {
	cm, err := reader.GetResource(ctx, resources.ConfigMap, name, client.InNamespace(namespace))
	if err != nil {
		return []string{fmt.Sprintf("failed to get gateway ConfigMap %q: %v", name, err)}
	}

	if cm == nil {
		return []string{fmt.Sprintf("gateway ConfigMap %q not found", name)}
	}

	return nil
}

// newCRConfigCondition creates a condition for CR spec validation results.
func (c *ImpactedWorkloadsCheck) newCRConfigCondition(total int, issueCount int, issues string) result.Condition {
	if total == 0 {
		return check.NewCondition(
			ConditionTypeOrchestratorCRConfigured,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No GuardrailsOrchestrators found - no CR configuration issues",
		)
	}

	if issueCount == 0 {
		return check.NewCondition(
			ConditionTypeOrchestratorCRConfigured,
			metav1.ConditionTrue,
			check.ReasonConfigurationValid,
			"All %d GuardrailsOrchestrator CR(s) have valid spec configuration",
			total,
		)
	}

	return check.NewCondition(
		ConditionTypeOrchestratorCRConfigured,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"%d of %d GuardrailsOrchestrator(s) have CR spec issues: %s",
		issueCount, total, issues,
		check.WithImpact(result.ImpactAdvisory),
	)
}

// newOrchestratorCMCondition creates a condition for orchestrator ConfigMap validation results.
func (c *ImpactedWorkloadsCheck) newOrchestratorCMCondition(total int, issueCount int, issues string) result.Condition {
	if total == 0 {
		return check.NewCondition(
			ConditionTypeOrchestratorConfigMapValid,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No GuardrailsOrchestrators found - no orchestrator ConfigMap issues",
		)
	}

	if issueCount == 0 {
		return check.NewCondition(
			ConditionTypeOrchestratorConfigMapValid,
			metav1.ConditionTrue,
			check.ReasonConfigurationValid,
			"All %d GuardrailsOrchestrator(s) have valid orchestrator ConfigMap configuration",
			total,
		)
	}

	return check.NewCondition(
		ConditionTypeOrchestratorConfigMapValid,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"%d of %d GuardrailsOrchestrator(s) have orchestrator ConfigMap issues: %s",
		issueCount, total, issues,
		check.WithImpact(result.ImpactAdvisory),
	)
}

// newGatewayCMCondition creates a condition for gateway ConfigMap validation results.
func (c *ImpactedWorkloadsCheck) newGatewayCMCondition(total int, issueCount int, issues string) result.Condition {
	if total == 0 {
		return check.NewCondition(
			ConditionTypeGatewayConfigMapValid,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No GuardrailsOrchestrators found - no gateway ConfigMap issues",
		)
	}

	if issueCount == 0 {
		return check.NewCondition(
			ConditionTypeGatewayConfigMapValid,
			metav1.ConditionTrue,
			check.ReasonConfigurationValid,
			"All %d GuardrailsOrchestrator(s) have valid gateway ConfigMap",
			total,
		)
	}

	return check.NewCondition(
		ConditionTypeGatewayConfigMapValid,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"%d of %d GuardrailsOrchestrator(s) have gateway ConfigMap issues: %s",
		issueCount, total, issues,
		check.WithImpact(result.ImpactAdvisory),
	)
}

// appendImpactedObject adds a GuardrailsOrchestrator to the impacted objects list.
func (c *ImpactedWorkloadsCheck) appendImpactedObject(dr *result.DiagnosticResult, obj *unstructured.Unstructured) {
	dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
		TypeMeta: resources.GuardrailsOrchestrator.TypeMeta(),
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		},
	})
}

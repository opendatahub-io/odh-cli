package llamastack

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind = "llamastackdistribution"

	ConditionTypeVLLMConfigured      = "VLLMConfigured"
	ConditionTypePostgresConfigured  = "PostgresConfigured"
	ConditionTypeEmbeddingConfigured = "EmbeddingConfigured"
	ConditionTypeConfigMapValid      = "ConfigMapValid"
)

// ConfigCheck validates LlamaStackDistribution resources for 3.3 upgrade compatibility.
type ConfigCheck struct {
	check.BaseCheck
}

func NewConfigCheck() *ConfigCheck {
	return &ConfigCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             "config",
			CheckID:          "workloads.llamastack.config",
			CheckName:        "Workloads :: LlamaStack :: Configuration (3.3)",
			CheckDescription: "Validates LlamaStackDistribution resources for required configuration changes in RHOAI 3.3",
			CheckRemediation: "Update LlamaStackDistribution CRs with required environment variables before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
func (c *ConfigCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "llamastackoperator", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ConfigCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.LlamaStackDistribution).
		Run(ctx, c.validateDistributions)
}

func (c *ConfigCheck) validateDistributions(
	ctx context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) error {
	count := len(req.Items)

	if count == 0 {
		// No LlamaStackDistributions found - nothing to validate
		req.Result.SetCondition(newNoWorkloadsCondition())

		return nil
	}

	// Track validation results across all LLSDs
	var (
		missingVLLMURL         []string
		missingPostgresHost    []string
		missingPostgresPass    []string
		missingEmbeddingURL    []string
		invalidConfigMaps      []string
		hasBedrockOldFormat    []string
		hasDeprecatedTelemetry []string
	)

	// Track impacted resources with specific issues per LLSD
	impactedMap := make(map[types.NamespacedName][]string)

	// Validate each LlamaStackDistribution
	for _, llsd := range req.Items {
		namespace := llsd.GetNamespace()
		name := llsd.GetName()
		key := fmt.Sprintf("%s/%s", namespace, name)
		nsName := types.NamespacedName{Namespace: namespace, Name: name}

		env, err := getEnvVars(llsd)
		if err != nil {
			return fmt.Errorf("getting env vars for %s: %w", key, err)
		}

		// Check required VLLM configuration
		if !hasEnvVar(env, "VLLM_URL") {
			missingVLLMURL = append(missingVLLMURL, key)
			impactedMap[nsName] = append(impactedMap[nsName], "missing-vllm-url")
		}

		// Check required PostgreSQL configuration
		if !hasEnvVar(env, "POSTGRES_HOST") {
			missingPostgresHost = append(missingPostgresHost, key)
			impactedMap[nsName] = append(impactedMap[nsName], "missing-postgres-host")
		}
		if !hasEnvVar(env, "POSTGRES_PASSWORD") {
			missingPostgresPass = append(missingPostgresPass, key)
			impactedMap[nsName] = append(impactedMap[nsName], "missing-postgres-password")
		}

		// Check required embedding configuration
		if !hasEnvVar(env, "VLLM_EMBEDDING_URL") {
			missingEmbeddingURL = append(missingEmbeddingURL, key)
			impactedMap[nsName] = append(impactedMap[nsName], "missing-embedding-url")
		}

		// Check ConfigMap if configured
		if err := validateConfigMap(ctx, req.Client, llsd); err != nil {
			invalidConfigMaps = append(invalidConfigMaps, fmt.Sprintf("%s: %s", key, err.Error()))
			impactedMap[nsName] = append(impactedMap[nsName], "invalid-configmap")
		}

		// Check for old Bedrock format (advisory)
		if hasOldBedrockFormat(env) && !hasEnvVar(env, "AWS_BEARER_TOKEN_BEDROCK") {
			hasBedrockOldFormat = append(hasBedrockOldFormat, key)
			impactedMap[nsName] = append(impactedMap[nsName], "deprecated-bedrock-format")
		}

		// Check for deprecated telemetry variables (advisory)
		if hasDeprecatedTelemetryVars(env) {
			hasDeprecatedTelemetry = append(hasDeprecatedTelemetry, key)
			impactedMap[nsName] = append(impactedMap[nsName], "deprecated-telemetry-vars")
		}
	}

	// Create conditions based on validation results
	conditions := c.buildConditions(
		missingVLLMURL,
		missingPostgresHost,
		missingPostgresPass,
		missingEmbeddingURL,
		invalidConfigMaps,
		hasBedrockOldFormat,
		hasDeprecatedTelemetry,
		count,
	)

	for _, cond := range conditions {
		req.Result.SetCondition(cond)
	}

	// Manually populate impacted objects with annotations describing specific issues
	// Initialize to empty slice (not nil) to prevent auto-population by the builder
	req.Result.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0)

	for nsName, issues := range impactedMap {
		// Only add objects that actually have issues
		if len(issues) > 0 {
			req.Result.ImpactedObjects = append(req.Result.ImpactedObjects, metav1.PartialObjectMetadata{
				TypeMeta: resources.LlamaStackDistribution.TypeMeta(),
				ObjectMeta: metav1.ObjectMeta{
					Namespace: nsName.Namespace,
					Name:      nsName.Name,
					Annotations: map[string]string{
						"llsd.issues": strings.Join(issues, ","),
					},
				},
			})
		}
	}

	return nil
}

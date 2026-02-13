package llamastack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// getEnvVars extracts environment variables from a LlamaStackDistribution.
func getEnvVars(llsd *unstructured.Unstructured) ([]map[string]any, error) {
	env, err := jq.Query[[]map[string]any](llsd, ".spec.server.containerSpec.env")
	if errors.Is(err, jq.ErrNotFound) {
		return []map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}

	return env, nil
}

// hasEnvVar checks if an environment variable with the given name exists.
func hasEnvVar(env []map[string]any, name string) bool {
	for _, e := range env {
		if envName, ok := e["name"].(string); ok && envName == name {
			return true
		}
	}

	return false
}

// hasOldBedrockFormat checks if any old AWS Bedrock env vars are present.
func hasOldBedrockFormat(env []map[string]any) bool {
	oldVars := []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE"}
	for _, varName := range oldVars {
		if hasEnvVar(env, varName) {
			return true
		}
	}

	return false
}

// hasDeprecatedTelemetryVars checks if deprecated telemetry vars are present.
func hasDeprecatedTelemetryVars(env []map[string]any) bool {
	deprecatedVars := []string{"OTEL_SERVICE_NAME", "TELEMETRY_SINKS", "OTEL_EXPORTER_OTLP_ENDPOINT"}
	for _, varName := range deprecatedVars {
		if hasEnvVar(env, varName) {
			return true
		}
	}

	return false
}

// validateConfigMap checks if the referenced ConfigMap exists and contains required keys.
func validateConfigMap(ctx context.Context, r client.Reader, llsd *unstructured.Unstructured) error {
	configMapName, err := jq.Query[string](llsd, ".spec.server.userConfig.configMapName")
	if errors.Is(err, jq.ErrNotFound) {
		return nil // No ConfigMap configured - OK
	}
	if err != nil {
		return fmt.Errorf("querying configMapName: %w", err)
	}

	namespace := llsd.GetNamespace()

	// We need the full object to check data, so list and filter
	configMaps, err := r.List(ctx, resources.ConfigMap)
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps: %w", err)
	}

	var targetCM *unstructured.Unstructured
	for _, obj := range configMaps {
		if obj.GetName() == configMapName && obj.GetNamespace() == namespace {
			targetCM = obj

			break
		}
	}

	if targetCM == nil {
		return fmt.Errorf("ConfigMap '%s' not found in namespace '%s'", configMapName, namespace)
	}

	// Check for config.yaml (required in 3.3)
	data, err := jq.Query[map[string]any](targetCM, ".data")
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap data: %w", err)
	}

	configYaml, hasConfigYaml := data["config.yaml"]
	if !hasConfigYaml {
		return errors.New("ConfigMap missing config.yaml (required for 3.3)")
	}

	// Validate config.yaml is parseable YAML
	configContent, ok := configYaml.(string)
	if !ok {
		return errors.New("config.yaml value is not a string")
	}

	if err := validateYAMLSyntax(configContent); err != nil {
		return fmt.Errorf("config.yaml is not parseable YAML: %w", err)
	}

	return nil
}

// validateYAMLSyntax attempts to parse YAML content to verify it's valid.
func validateYAMLSyntax(content string) error {
	var data any
	if err := yaml.Unmarshal([]byte(content), &data); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	return nil
}

func newNoWorkloadsCondition() result.Condition {
	return check.NewCondition(
		ConditionTypeVLLMConfigured,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonResourceNotFound),
		check.WithMessage("No LlamaStackDistribution resources found"),
	)
}

func (c *ConfigCheck) buildConditions(
	missingVLLMURL []string,
	missingPostgresHost []string,
	missingPostgresPass []string,
	missingEmbeddingURL []string,
	invalidConfigMaps []string,
	hasBedrockOldFormat []string,
	hasDeprecatedTelemetry []string,
	totalCount int,
) []result.Condition {
	var conditions []result.Condition

	// VLLM_URL validation (blocking)
	if len(missingVLLMURL) > 0 {
		conditions = append(conditions, check.NewCondition(
			ConditionTypeVLLMConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("%d LlamaStackDistribution(s) missing required VLLM_URL environment variable", len(missingVLLMURL)),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation("Add VLLM_URL to spec.server.containerSpec.env for all LlamaStackDistributions"),
		))
	} else {
		conditions = append(conditions, check.NewCondition(
			ConditionTypeVLLMConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("All %d LlamaStackDistribution(s) have VLLM_URL configured", totalCount),
		))
	}

	// PostgreSQL validation (blocking)
	pgMissingCount := len(missingPostgresHost) + len(missingPostgresPass)
	if pgMissingCount > 0 {
		// Build message parts
		var msgParts []string
		if len(missingPostgresHost) > 0 {
			msgParts = append(msgParts, fmt.Sprintf("%d missing POSTGRES_HOST", len(missingPostgresHost)))
		}
		if len(missingPostgresPass) > 0 {
			msgParts = append(msgParts, fmt.Sprintf("%d missing POSTGRES_PASSWORD", len(missingPostgresPass)))
		}
		msg := "LlamaStackDistribution(s) missing required PostgreSQL configuration: " + msgParts[0]
		if len(msgParts) > 1 {
			msg += ", " + msgParts[1]
		}

		cond := check.NewCondition(
			ConditionTypePostgresConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation("Add POSTGRES_HOST and POSTGRES_PASSWORD to spec.server.containerSpec.env for all LlamaStackDistributions"),
		)
		cond.Message = msg
		conditions = append(conditions, cond)
	} else {
		conditions = append(conditions, check.NewCondition(
			ConditionTypePostgresConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("All %d LlamaStackDistribution(s) have PostgreSQL configured", totalCount),
		))
	}

	// Embedding model validation (blocking)
	if len(missingEmbeddingURL) > 0 {
		conditions = append(conditions, check.NewCondition(
			ConditionTypeEmbeddingConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("%d LlamaStackDistribution(s) missing required VLLM_EMBEDDING_URL environment variable", len(missingEmbeddingURL)),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation("Add VLLM_EMBEDDING_URL to spec.server.containerSpec.env for all LlamaStackDistributions"),
		))
	} else {
		conditions = append(conditions, check.NewCondition(
			ConditionTypeEmbeddingConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("All %d LlamaStackDistribution(s) have embedding model configured", totalCount),
		))
	}

	// ConfigMap validation (blocking)
	if len(invalidConfigMaps) > 0 {
		// Build detailed message with specific errors
		msg := fmt.Sprintf("%d LlamaStackDistribution(s) have invalid ConfigMap configuration: ", len(invalidConfigMaps))
		var msgSb228 strings.Builder
		for i, errMsg := range invalidConfigMaps {
			if i > 0 {
				_, _ = msgSb228.WriteString("; ")
			}
			_, _ = msgSb228.WriteString(errMsg)
		}
		msg += msgSb228.String()

		conditions = append(conditions, check.NewCondition(
			ConditionTypeConfigMapValid,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation("Fix ConfigMap issues: ensure ConfigMap exists, contains config.yaml key, and YAML syntax is valid"),
		))
		// Set message directly to include detailed errors
		conditions[len(conditions)-1].Message = msg
	} else {
		conditions = append(conditions, check.NewCondition(
			ConditionTypeConfigMapValid,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("All ConfigMaps are valid YAML - verify config.yaml content against llamastack documentation"),
		))
	}

	// AWS Bedrock configuration validation (blocking)
	if len(hasBedrockOldFormat) > 0 {
		conditions = append(conditions, check.NewCondition(
			"BedrockConfigCompatible",
			metav1.ConditionFalse,
			check.WithReason("ConfigurationDeprecated"),
			check.WithMessage("%d LlamaStackDistribution(s) use deprecated AWS Bedrock configuration format - migrate to AWS_BEARER_TOKEN_BEDROCK", len(hasBedrockOldFormat)),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation("Replace AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN, and AWS_PROFILE with AWS_BEARER_TOKEN_BEDROCK"),
		))
	}

	if len(hasDeprecatedTelemetry) > 0 {
		conditions = append(conditions, check.NewCondition(
			"TelemetryConfigCompatible",
			metav1.ConditionFalse,
			check.WithReason("ConfigurationDeprecated"),
			check.WithMessage("%d LlamaStackDistribution(s) use deprecated telemetry variables - these will be ignored in 3.3", len(hasDeprecatedTelemetry)),
			check.WithImpact(result.ImpactAdvisory),
		))
	}

	return conditions
}

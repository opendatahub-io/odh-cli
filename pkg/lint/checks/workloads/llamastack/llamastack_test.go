package llamastack_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.LlamaStackDistribution.GVR(): resources.LlamaStackDistribution.ListKind(),
	resources.ConfigMap.GVR():              resources.ConfigMap.ListKind(),
	resources.DataScienceCluster.GVR():     resources.DataScienceCluster.ListKind(),
}

func newDSC(componentStates map[string]string) *unstructured.Unstructured {
	components := make(map[string]any, len(componentStates))
	for name, state := range componentStates {
		components[name] = map[string]any{
			"managementState": state,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": components,
			},
		},
	}
}

func newLLSD(name, namespace string, envVars map[string]string, configMapName string) *unstructured.Unstructured {
	env := make([]any, 0, len(envVars))
	for k, v := range envVars {
		env = append(env, map[string]any{
			"name":  k,
			"value": v,
		})
	}

	spec := map[string]any{
		"server": map[string]any{
			"containerSpec": map[string]any{
				"env": env,
			},
		},
	}

	if configMapName != "" {
		spec["server"].(map[string]any)["userConfig"] = map[string]any{
			"configMapName": configMapName,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.LlamaStackDistribution.APIVersion(),
			"kind":       resources.LlamaStackDistribution.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func newConfigMap(name, namespace string, hasConfigYaml bool) *unstructured.Unstructured {
	return newConfigMapWithContent(name, namespace, hasConfigYaml, "# valid YAML config content\nkey: value")
}

func newConfigMapWithContent(
	name string,
	namespace string,
	hasConfigYaml bool,
	content string,
) *unstructured.Unstructured {
	data := map[string]any{}
	if hasConfigYaml {
		data["config.yaml"] = content
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}
}

func TestLlamaStackConfigCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name           string
		currentVersion string
		targetVersion  string
		componentState string
		expected       bool
	}{
		{
			name:           "2.25 to 3.3 upgrade with component Managed",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "2.17 to 3.0 upgrade with component Managed",
			currentVersion: "2.17.0",
			targetVersion:  "3.0.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "3.0 to 3.3 upgrade (wrong version range)",
			currentVersion: "3.0.0",
			targetVersion:  "3.3.0",
			componentState: "Managed",
			expected:       false,
		},
		{
			name:           "2.25 to 3.3 upgrade with component Removed",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Removed",
			expected:       false,
		},
		{
			name:           "2.25 to 3.3 upgrade with component Unmanaged",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Unmanaged",
			expected:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := newDSC(map[string]string{"llamastackoperator": tc.componentState})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			chk := llamastack.NewConfigCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestLlamaStackConfigCheck_NoWorkloads(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("VLLMConfigured"),
		"Status": Equal(metav1.ConditionTrue),
	}))
}

func TestLlamaStackConfigCheck_AllRequiredConfigured(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"VLLM_URL":           "http://vllm:8000",
		"POSTGRES_HOST":      "postgres.default.svc",
		"POSTGRES_PASSWORD":  "password",
		"VLLM_EMBEDDING_URL": "http://embedding:8000",
	}

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMap("test-config", "test-ns", true)
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(4))

	// All conditions should be True
	for _, cond := range res.Status.Conditions {
		g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		g.Expect(cond.Impact).To(Equal(result.ImpactNone))
	}

	// Verify ConfigMapValid condition mentions verification
	var cmCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "ConfigMapValid" {
			cmCond = &res.Status.Conditions[i]

			break
		}
	}
	g.Expect(cmCond).ToNot(BeNil())
	g.Expect(cmCond.Message).To(ContainSubstring("verify config.yaml content against llamastack documentation"))
}

func TestLlamaStackConfigCheck_MissingVLLMURL(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"POSTGRES_HOST":      "postgres.default.svc",
		"POSTGRES_PASSWORD":  "password",
		"VLLM_EMBEDDING_URL": "http://embedding:8000",
	}

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMap("test-config", "test-ns", true)
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())

	// Find VLLMConfigured condition
	var vllmCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "VLLMConfigured" {
			vllmCond = &res.Status.Conditions[i]

			break
		}
	}

	g.Expect(vllmCond).ToNot(BeNil())
	g.Expect(vllmCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(vllmCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(vllmCond.Message).To(ContainSubstring("missing required VLLM_URL"))
}

func TestLlamaStackConfigCheck_MissingPostgresConfig(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"VLLM_URL":           "http://vllm:8000",
		"VLLM_EMBEDDING_URL": "http://embedding:8000",
	}

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMap("test-config", "test-ns", true)
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())

	// Find PostgresConfigured condition
	var pgCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "PostgresConfigured" {
			pgCond = &res.Status.Conditions[i]

			break
		}
	}

	g.Expect(pgCond).ToNot(BeNil())
	g.Expect(pgCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(pgCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(pgCond.Message).To(ContainSubstring("PostgreSQL configuration"))
}

func TestLlamaStackConfigCheck_InvalidConfigMap(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"VLLM_URL":           "http://vllm:8000",
		"POSTGRES_HOST":      "postgres.default.svc",
		"POSTGRES_PASSWORD":  "password",
		"VLLM_EMBEDDING_URL": "http://embedding:8000",
	}

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMap("test-config", "test-ns", false) // Missing config.yaml
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())

	// Find ConfigMapValid condition
	var cmCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "ConfigMapValid" {
			cmCond = &res.Status.Conditions[i]

			break
		}
	}

	g.Expect(cmCond).ToNot(BeNil())
	g.Expect(cmCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cmCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(cmCond.Message).To(ContainSubstring("missing config.yaml"))
}

func TestLlamaStackConfigCheck_InvalidYAMLSyntax(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"VLLM_URL":           "http://vllm:8000",
		"POSTGRES_HOST":      "postgres.default.svc",
		"POSTGRES_PASSWORD":  "password",
		"VLLM_EMBEDDING_URL": "http://embedding:8000",
	}

	// ConfigMap with invalid YAML syntax
	invalidYAML := "this is not valid YAML: [\n  unclosed bracket"
	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMapWithContent("test-config", "test-ns", true, invalidYAML)
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())

	// Find ConfigMapValid condition
	var cmCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "ConfigMapValid" {
			cmCond = &res.Status.Conditions[i]

			break
		}
	}

	g.Expect(cmCond).ToNot(BeNil())
	g.Expect(cmCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cmCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(cmCond.Message).To(ContainSubstring("not parseable YAML"))
}

func TestLlamaStackConfigCheck_DeprecatedBedrockFormat(t *testing.T) {
	g := NewWithT(t)

	envVars := map[string]string{
		"VLLM_URL":              "http://vllm:8000",
		"POSTGRES_HOST":         "postgres.default.svc",
		"POSTGRES_PASSWORD":     "password",
		"VLLM_EMBEDDING_URL":    "http://embedding:8000",
		"AWS_ACCESS_KEY_ID":     "old-key-id", // Deprecated format
		"AWS_SECRET_ACCESS_KEY": "old-secret",
	}

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	cm := newConfigMap("test-config", "test-ns", true)
	llsd := newLLSD("test-llsd", "test-ns", envVars, "test-config")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd, cm},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())

	// Find BedrockConfigCompatible condition
	var bedrockCond *result.Condition
	for i := range res.Status.Conditions {
		if res.Status.Conditions[i].Type == "BedrockConfigCompatible" {
			bedrockCond = &res.Status.Conditions[i]

			break
		}
	}

	g.Expect(bedrockCond).ToNot(BeNil())
	g.Expect(bedrockCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(bedrockCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(bedrockCond.Message).To(ContainSubstring("deprecated AWS Bedrock configuration"))
}

func TestLlamaStackConfigCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := llamastack.NewConfigCheck()

	g.Expect(chk.ID()).To(Equal("workloads.llamastack.config"))
	g.Expect(chk.Name()).To(Equal("Workloads :: LlamaStack :: Configuration (3.3)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

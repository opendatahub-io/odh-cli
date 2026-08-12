package resources_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

func TestComponentCRResourceTypes(t *testing.T) {
	expectedComponents := []string{
		"aigateway",
		"aipipelines",
		"dashboard",
		"feastoperator",
		"kserve",
		"llamastackoperator",
		"mlflowoperator",
		"modelregistry",
		"ray",
		"sparkoperator",
		"trainer",
		"trainingoperator",
		"trustyai",
		"workbenches",
	}

	for _, name := range expectedComponents {
		t.Run(name, func(t *testing.T) {
			rt, ok := resources.ComponentCRResourceTypes[name]
			if !ok {
				t.Fatalf("component %q not found in ComponentCRResourceTypes", name)
			}

			if rt.Group != "components.platform.opendatahub.io" {
				t.Errorf("group = %q, want components.platform.opendatahub.io", rt.Group)
			}

			if rt.Version != "v1alpha1" {
				t.Errorf("version = %q, want v1alpha1", rt.Version)
			}

			if rt.Kind == "" {
				t.Error("kind should not be empty")
			}

			if rt.Resource == "" {
				t.Error("resource should not be empty")
			}
		})
	}
}

func TestGetComponentLabelValue(t *testing.T) {
	tests := []struct {
		component string
		want      string
	}{
		{"kserve", "kserve"},
		{"dashboard", "dashboard"},
		{"aipipelines", "data-science-pipelines-operator"},
		{"modelregistry", "model-registry-operator"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.component, func(t *testing.T) {
			got := resources.GetComponentLabelValue(tt.component)
			if got != tt.want {
				t.Errorf("GetComponentLabelValue(%q) = %q, want %q", tt.component, got, tt.want)
			}
		})
	}
}

// TestAllComponentsHaveVerifiedLabels ensures every component in ComponentCRResourceTypes
// has an explicitly verified label value. Adding a new component requires adding its
// expected label here, forcing the developer to verify the actual app.kubernetes.io/part-of
// label used by that component's resources.
func TestAllComponentsHaveVerifiedLabels(t *testing.T) {
	// expectedLabels maps component names to their verified app.kubernetes.io/part-of label values.
	// When adding a new component, verify its actual label by checking deployed resources:
	//   oc get pods -n opendatahub -l app.kubernetes.io/part-of=<component> --show-labels
	expectedLabels := map[string]string{
		"aigateway":          "aigateway",
		"aipipelines":        "data-science-pipelines-operator",
		"dashboard":          "dashboard",
		"feastoperator":      "feastoperator",
		"kserve":             "kserve",
		"llamastackoperator": "llamastackoperator",
		"mlflowoperator":     "mlflowoperator",
		"modelregistry":      "model-registry-operator",
		"ray":                "ray",
		"sparkoperator":      "sparkoperator",
		"trainer":            "trainer",
		"trainingoperator":   "trainingoperator",
		"trustyai":           "trustyai",
		"workbenches":        "workbenches",
	}

	// Check that every component in ComponentCRResourceTypes has a verified label
	for name := range resources.ComponentCRResourceTypes {
		t.Run(name, func(t *testing.T) {
			expected, ok := expectedLabels[name]
			if !ok {
				t.Fatalf("component %q missing from expectedLabels - verify its app.kubernetes.io/part-of label and add it", name)
			}

			got := resources.GetComponentLabelValue(name)
			if got != expected {
				t.Errorf("GetComponentLabelValue(%q) = %q, want %q", name, got, expected)
			}
		})
	}

	// Check that expectedLabels doesn't have stale entries
	for name := range expectedLabels {
		if _, ok := resources.ComponentCRResourceTypes[name]; !ok {
			t.Errorf("expectedLabels has %q but it's not in ComponentCRResourceTypes - remove stale entry", name)
		}
	}
}

func TestGetComponentCR(t *testing.T) {
	tests := []struct {
		name      string
		component string
		wantNil   bool
		wantKind  string
	}{
		{
			name:      "returns ResourceType for known component",
			component: "kserve",
			wantNil:   false,
			wantKind:  "Kserve",
		},
		{
			name:      "returns ResourceType for dashboard",
			component: "dashboard",
			wantNil:   false,
			wantKind:  "Dashboard",
		},
		{
			name:      "returns nil for unknown component",
			component: "unknown",
			wantNil:   true,
		},
		{
			name:      "returns nil for empty string",
			component: "",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resources.GetComponentCR(tt.component)

			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}

				return
			}

			if got == nil {
				t.Fatal("expected non-nil ResourceType")

				return
			}

			if got.Kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", got.Kind, tt.wantKind)
			}
		})
	}
}

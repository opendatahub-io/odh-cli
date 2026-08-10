package aipipelines

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

//nolint:gochecknoglobals // Test exports only compiled in test builds
var (
	GetPodGroup                 = getPodGroup
	ClassifyRole                = classifyRole
	IsSystemNamespace           = isSystemNamespace
	DefaultStatePath            = defaultStatePath
	SavePodHealthState          = savePodHealthState
	LoadPodHealthState          = loadPodHealthState
	HasAnyDegradation           = hasAnyDegradation
	ExtractStringSlice          = extractStringSlice
	ListDSPAs                   = listDSPAs
	HasV1Alpha1StoredVersion    = hasV1Alpha1StoredVersion
	RemoveV1Alpha1StoredVersion = removeV1Alpha1StoredVersion
	CaptureAndSavePodHealth     = captureAndSavePodHealth
)

// SetDefaultStatePath overrides the state file path for testing.
// Returns a cleanup function that restores the original.
func SetDefaultStatePath(path string) func() {
	original := defaultStatePathFunc

	defaultStatePathFunc = func() string { return path }

	return func() { defaultStatePathFunc = original }
}

func MakePodUnstructured(name, namespace, phase, readyStatus string) unstructured.Unstructured {
	conditions := []any{}
	if readyStatus != "" {
		conditions = append(conditions, map[string]any{
			"type":   "Ready",
			"status": readyStatus,
		})
	}

	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]any{
				"phase":      phase,
				"conditions": conditions,
			},
		},
	}
}

func MakeRoleUnstructured(name, namespace string, rules []map[string]any) unstructured.Unstructured {
	rulesAny := make([]any, len(rules))
	for i, r := range rules {
		rulesAny[i] = r
	}

	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"rules": rulesAny,
		},
	}
}

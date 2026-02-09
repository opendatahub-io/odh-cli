package testutil

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
)

// TargetConfig holds all parameters needed to build a check.Target for tests.
type TargetConfig struct {
	ListKinds      map[schema.GroupVersionResource]string
	Objects        []*unstructured.Unstructured
	CurrentVersion string
	TargetVersion  string
}

// NewTarget builds a check.Target from fake clients, reducing test boilerplate.
// Objects are automatically registered in both the dynamic and metadata fake clients.
func NewTarget(t *testing.T, cfg TargetConfig) check.Target {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicObjs := make([]runtime.Object, len(cfg.Objects))
	for i, obj := range cfg.Objects {
		dynamicObjs[i] = obj
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		cfg.ListKinds,
		dynamicObjs...,
	)

	metadataClient := metadatafake.NewSimpleMetadataClient(
		scheme,
		kube.ToPartialObjectMetadata(cfg.Objects...)...,
	)

	target := check.Target{
		Client: client.NewForTesting(client.TestClientConfig{
			Dynamic:  dynamicClient,
			Metadata: metadataClient,
		}),
	}

	if cfg.CurrentVersion != "" {
		v := semver.MustParse(cfg.CurrentVersion)
		target.CurrentVersion = &v
	}

	if cfg.TargetVersion != "" {
		v := semver.MustParse(cfg.TargetVersion)
		target.TargetVersion = &v
	}

	return target
}

// NewDSCI creates an unstructured DSCInitialization object for tests.
func NewDSCI(applicationsNamespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": applicationsNamespace,
			},
		},
	}
}

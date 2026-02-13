package validate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var notebookListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR(): resources.Notebook.ListKind(),
}

//nolint:gochecknoglobals // Test fixture - shared across test functions
var pytorchJobListKinds = map[schema.GroupVersionResource]string{
	resources.PyTorchJob.GVR(): resources.PyTorchJob.ListKind(),
}

func newWorkloadTestCheck() *testCheck {
	return &testCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             "notebook",
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "test.workload.check",
			CheckName:        "Test Workload Check",
			CheckDescription: "Test workload description",
		},
	}
}

func TestWorkloadBuilder_MetadataListing_NoFilter_AutoPopulate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   map[string]any{"name": "nb-1", "namespace": "ns1"},
		},
	}

	nb2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   map[string]any{"name": "nb-2", "namespace": "ns2"},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds, nb1, nb2)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, kube.ToPartialObjectMetadata(nb1, nb2)...)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	chk := newWorkloadTestCheck()
	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			g.Expect(req.Items).To(HaveLen(2))
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Found %d notebooks", len(req.Items)),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Group).To(Equal("workload"))
	g.Expect(dr.Kind).To(Equal("notebook"))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))

	// Auto-populated ImpactedObjects.
	g.Expect(dr.ImpactedObjects).To(HaveLen(2))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("nb-1"))
	g.Expect(dr.ImpactedObjects[1].Name).To(Equal("nb-2"))
}

func TestWorkloadBuilder_FullObjectListing_WithFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	job1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata":   map[string]any{"name": "job-match", "namespace": "ns1"},
			"spec":       map[string]any{"keep": true},
		},
	}

	job2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata":   map[string]any{"name": "job-skip", "namespace": "ns2"},
			"spec":       map[string]any{"keep": false},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, pytorchJobListKinds, job1, job2)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	chk := newWorkloadTestCheck()
	chk.Kind = "trainingoperator"

	targetVer := semver.MustParse("3.3.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	dr, err := validate.Workloads(chk, target, resources.PyTorchJob).
		Filter(func(obj *unstructured.Unstructured) (bool, error) {
			return obj.GetName() == "job-match", nil
		}).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*unstructured.Unstructured]) error {
			g.Expect(req.Items).To(HaveLen(1))
			g.Expect(req.Items[0].GetName()).To(Equal("job-match"))
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Filtered to %d jobs", len(req.Items)),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))

	// Auto-populated ImpactedObjects for filtered items.
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("job-match"))
}

func TestWorkloadBuilder_FilterError_Propagated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	job := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata":   map[string]any{"name": "job-1", "namespace": "ns1"},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, pytorchJobListKinds, job)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	filterErr := errors.New("filter failed")

	_, err := validate.Workloads(chk, target, resources.PyTorchJob).
		Filter(func(_ *unstructured.Unstructured) (bool, error) {
			return false, filterErr
		}).
		Run(ctx, func(_ context.Context, _ *validate.WorkloadRequest[*unstructured.Unstructured]) error {
			return nil
		})

	g.Expect(err).To(HaveOccurred())
	g.Expect(err).To(MatchError(ContainSubstring("filtering")))
	g.Expect(err).To(MatchError(ContainSubstring("filter failed")))
}

func TestWorkloadBuilder_CRDNotFound_EmptyItems(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	validationCalled := false

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			validationCalled = true
			g.Expect(req.Items).To(BeEmpty())
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("No notebooks found"),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(validationCalled).To(BeTrue())
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(dr.ImpactedObjects).To(BeEmpty())
}

func TestWorkloadBuilder_CustomImpactedObjects_SkipsAutoPopulate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   map[string]any{"name": "nb-1", "namespace": "ns1"},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds, nb)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, kube.ToPartialObjectMetadata(nb)...)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Custom objects"),
			))

			// Set custom ImpactedObjects — prevents auto-population.
			req.Result.ImpactedObjects = []metav1.PartialObjectMetadata{
				{
					TypeMeta: resources.Notebook.TypeMeta(),
					ObjectMeta: metav1.ObjectMeta{
						Name:      "nb-1",
						Namespace: "ns1",
						Annotations: map[string]string{
							"custom-key": "custom-value",
						},
					},
				},
			}

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())

	// Custom ImpactedObjects preserved — not overwritten by auto-populate.
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))
	g.Expect(dr.ImpactedObjects[0].Annotations).To(HaveKeyWithValue("custom-key", "custom-value"))
}

func TestWorkloadBuilder_ErrorFromMapper(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	expectedErr := errors.New("mapper failed")
	_, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, _ *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			return expectedErr
		})

	g.Expect(err).To(MatchError(expectedErr))
}

func TestWorkloadBuilder_NoTargetVersion_AnnotationNotSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("No version check"),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Annotations).ToNot(HaveKey(check.AnnotationCheckTargetVersion))
}

func TestWorkloadBuilder_EmptyItems_NoAutoPopulate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			g.Expect(req.Items).To(BeEmpty())
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Empty"),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.ImpactedObjects).To(BeEmpty())
}

func TestWorkloadBuilder_ClientIsPassedToRequest(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			g.Expect(req.Client).ToNot(BeNil())
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Client present"),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
}

func TestWorkloadBuilder_ResultMetadata(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("Metadata check"),
			))

			return nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())

	// Verify result metadata matches the check.
	g.Expect(dr.Group).To(Equal("workload"))
	g.Expect(dr.Kind).To(Equal("notebook"))
	g.Expect(dr.Name).To(Equal(string(check.CheckTypeImpactedWorkloads)))
	g.Expect(dr.Spec.Description).To(Equal("Test workload description"))
}

func TestWorkloadBuilder_Complete_SetsConditions(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   map[string]any{"name": "nb-1", "namespace": "ns1"},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds, nb)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, kube.ToPartialObjectMetadata(nb)...)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &targetVer,
	}

	dr, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Complete(ctx, func(_ context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) ([]result.Condition, error) {
			return []result.Condition{
				check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Found %d notebooks", len(req.Items)),
				),
			}, nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeCompatible))
	g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(dr.Status.Conditions[0].Message).To(Equal("Found 1 notebooks"))
}

func TestWorkloadBuilder_Complete_ErrorPropagated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, notebookListKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	chk := newWorkloadTestCheck()
	target := check.Target{
		Client: c,
	}

	expectedErr := errors.New("condition fn failed")

	_, err := validate.WorkloadsMetadata(chk, target, resources.Notebook).
		Complete(ctx, func(_ context.Context, _ *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) ([]result.Condition, error) {
			return nil, expectedErr
		})

	g.Expect(err).To(MatchError(expectedErr))
}

package crd_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	platformcrd "github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/crd"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var webhookListKinds = map[schema.GroupVersionResource]string{
	resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
	resources.Service.GVR():                  resources.Service.ListKind(),
}

func newCRDWithWebhook(name, svcName, svcNamespace, strategy string) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.CustomResourceDefinition.APIVersion(),
		"kind":       resources.CustomResourceDefinition.Kind,
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"conversion": map[string]any{
				"strategy": strategy,
			},
		},
	}

	if strategy == "Webhook" {
		obj["spec"].(map[string]any)["conversion"].(map[string]any)["webhook"] = map[string]any{
			"clientConfig": map[string]any{
				"service": map[string]any{
					"name":      svcName,
					"namespace": svcNamespace,
				},
			},
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newService(name, namespace string) *unstructured.Unstructured {
	svc := &unstructured.Unstructured{}
	svc.SetGroupVersionKind(resources.Service.GVK())
	svc.SetName(name)
	svc.SetNamespace(namespace)

	return svc
}

func TestWebhookAvailabilityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := platformcrd.NewWebhookAvailabilityCheck()

	g.Expect(chk.ID()).To(Equal("platform.crd.webhook-availability"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestWebhookAvailabilityCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := platformcrd.NewWebhookAvailabilityCheck()

	// Should apply for 2.x -> 3.x upgrade
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: webhookListKinds,
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestWebhookAvailabilityCheck_WithWebhookHealthy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newCRDWithWebhook(
		"datascienceclusters.datasciencecluster.opendatahub.io",
		"webhook-service", "redhat-ods-operator", "Webhook",
	)
	svc := newService("webhook-service", "redhat-ods-operator")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		Objects:        []*unstructured.Unstructured{crd, svc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewWebhookAvailabilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("WebhookHealthy"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestWebhookAvailabilityCheck_WithWebhookServiceMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newCRDWithWebhook(
		"datascienceclusters.datasciencecluster.opendatahub.io",
		"webhook-service", "redhat-ods-operator", "Webhook",
	)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewWebhookAvailabilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("WebhookHealthy"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceUnavailable),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
}

func TestWebhookAvailabilityCheck_NoWebhookStrategy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newCRDWithWebhook(
		"datascienceclusters.datasciencecluster.opendatahub.io",
		"", "", "None",
	)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewWebhookAvailabilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("WebhookHealthy"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestWebhookAvailabilityCheck_CRDNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// No CRDs registered
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      webhookListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewWebhookAvailabilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("WebhookHealthy"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

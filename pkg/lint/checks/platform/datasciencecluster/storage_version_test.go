package datasciencecluster_test

import (
	"encoding/json"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/stretchr/testify/mock"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/datasciencecluster"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	mockclient "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/client"

	. "github.com/onsi/gomega"
)

const (
	storedV1   = `{"status":{"storedVersions":["v1"]}}`
	storedV1V2 = `{"status":{"storedVersions":["v1","v2"]}}`
	storedV1V3 = `{"status":{"storedVersions":["v3","v1"]}}`
	storedV2   = `{"status":{"storedVersions":["v2"]}}`
	storedV3   = `{"status":{"storedVersions":["v3"]}}`

	storedV2V3 = `{"status":{"storedVersions":["v2","v3"]}}`

	missingStored     = `{}`
	emptyStored       = `{"status":{"storedVersions":[]}}`
	malformedStored   = `{"status":{"storedVersions":"v1"}}`
	invalidStoredItem = `{"status":{"storedVersions":[12]}}`
	nullStoredItem    = `{"status":{"storedVersions":[null]}}`

	upgradeTarget    = "3.6.0"
	migrationCommand = "odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0"
)

func TestStorageVersionCheckBlocksV1(t *testing.T) {
	for name, fixture := range map[string]string{
		"v1 only":     storedV1,
		"v1 and v2":   storedV1V2,
		"v1 after v3": storedV1V3,
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			target := storageCheckTarget(t, fixture, nil)

			dr, err := datasciencecluster.NewStorageVersionCheck().Validate(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(dr.Validate()).To(Succeed())
			g.Expect(dr.Status.Conditions).To(HaveLen(1))
			g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, upgradeTarget))

			condition := dr.Status.Conditions[0]
			g.Expect(condition).To(HaveField("Status", Equal(metav1.ConditionFalse)))
			g.Expect(condition).To(HaveField("Impact", Equal(result.ImpactBlocking)))
			g.Expect(condition).To(HaveField("Remediation", ContainSubstring(migrationCommand)))
		})
	}
}

func TestStorageVersionCheckPassesWithoutV1(t *testing.T) {
	for name, fixture := range map[string]string{
		"v2 only":   storedV2,
		"v3 only":   storedV3,
		"v2 and v3": storedV2V3,
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			target := storageCheckTarget(t, fixture, nil)

			dr, err := datasciencecluster.NewStorageVersionCheck().Validate(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(dr.Validate()).To(Succeed())
			g.Expect(dr.Status.Conditions).To(HaveLen(1))

			condition := dr.Status.Conditions[0]
			g.Expect(condition).To(HaveField("Status", Equal(metav1.ConditionTrue)))
			g.Expect(condition).To(HaveField("Impact", Equal(result.ImpactNone)))
		})
	}
}

func TestStorageVersionCheckRejectsInvalidHistory(t *testing.T) {
	for name, fixture := range map[string]string{
		"nil CRD":                "",
		"missing status":         missingStored,
		"empty history":          emptyStored,
		"string instead of list": malformedStored,
		"numeric version":        invalidStoredItem,
		"null version":           nullStoredItem,
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			target := storageCheckTarget(t, fixture, nil)

			dr, err := datasciencecluster.NewStorageVersionCheck().Validate(t.Context(), target)

			g.Expect(err).To(HaveOccurred())
			g.Expect(dr).To(BeNil())
		})
	}
}

func TestStorageVersionCheckReportsReadErrors(t *testing.T) {
	crdResource := resources.CustomResourceDefinition.GVR().GroupResource()
	crdName := resources.DataScienceCluster.CRDFQN()

	for name, readErr := range map[string]error{
		"not found":          apierrors.NewNotFound(crdResource, crdName),
		"forbidden":          apierrors.NewForbidden(crdResource, crdName, nil),
		"server unavailable": apierrors.NewServiceUnavailable("unavailable"),
	} {
		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			target := storageCheckTarget(t, "", readErr)

			dr, err := datasciencecluster.NewStorageVersionCheck().Validate(t.Context(), target)

			g.Expect(err).To(MatchError(ContainSubstring(readErr.Error())))
			g.Expect(dr).To(BeNil())
		})
	}
}

func TestStorageVersionCheckCanApply(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		target  string
		applies bool
	}{
		{name: "missing target", source: "3.5.0"},
		{name: "target below 3.6", source: "2.25.0", target: "3.5.9"},
		{name: "target 3.6", source: "3.5.0", target: upgradeTarget, applies: true},
		{name: "later minor", source: "3.6.0", target: "3.7.0", applies: true},
		{name: "later major", source: "3.6.0", target: "4.0.0", applies: true},
		{name: "missing source", target: upgradeTarget, applies: true},
		{name: "same release", source: upgradeTarget, target: upgradeTarget, applies: true},
		{name: "newer source", source: "4.0.0", target: upgradeTarget, applies: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			target := check.Target{
				CurrentVersion: optionalVersion(tc.source),
				TargetVersion:  optionalVersion(tc.target),
			}

			applies, err := datasciencecluster.NewStorageVersionCheck().CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(applies).To(Equal(tc.applies))
		})
	}
}

func storageCheckTarget(t *testing.T, fixture string, readErr error) check.Target {
	t.Helper()

	var crd *unstructured.Unstructured
	if fixture != "" {
		crd = &unstructured.Unstructured{}
		NewWithT(t).Expect(json.Unmarshal([]byte(fixture), &crd.Object)).To(Succeed())
	}

	reader := &mockclient.MockClient{}
	reader.On(
		"GetResource",
		mock.Anything,
		resources.CustomResourceDefinition,
		resources.DataScienceCluster.CRDFQN(),
		mock.Anything,
	).Return(crd, readErr).Once()

	// Reject unexpected reads or lists, including any DSC v1 request.
	t.Cleanup(func() { reader.AssertExpectations(t) })

	return check.Target{Client: reader, TargetVersion: optionalVersion(upgradeTarget)}
}

func optionalVersion(value string) *semver.Version {
	if value == "" {
		return nil
	}

	v := semver.MustParse(value)

	return &v
}

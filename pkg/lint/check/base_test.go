package check_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

func TestBaseCheck(t *testing.T) {
	g := NewWithT(t)

	t.Run("should provide all metadata fields", func(t *testing.T) {
		bc := check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             "test-component",
			Type:             "test-type",
			CheckID:          "components.test.check",
			CheckName:        "Test Check Name",
			CheckDescription: "Test description",
		}

		// Interface methods
		g.Expect(bc.ID()).To(Equal("components.test.check"))
		g.Expect(bc.Name()).To(Equal("Test Check Name"))
		g.Expect(bc.Description()).To(Equal("Test description"))
		g.Expect(bc.Group()).To(Equal(check.GroupComponent))

		// Public fields (can be accessed directly)
		g.Expect(bc.Kind).To(Equal("test-component"))
		g.Expect(bc.Type).To(Equal(check.CheckType("test-type")))
	})

	t.Run("NewResult should create properly initialized result", func(t *testing.T) {
		bc := check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.kserve.removal",
			CheckName:        "KServe Removal",
			CheckDescription: "Validates KServe removal",
		}

		dr := bc.NewResult()

		g.Expect(dr.Group).To(Equal("component"))
		g.Expect(dr.Kind).To(Equal(constants.ComponentKServe))
		g.Expect(dr.Name).To(Equal(string(check.CheckTypeRemoval)))
		g.Expect(dr.Spec.Description).To(Equal("Validates KServe removal"))
		g.Expect(dr.Annotations).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).ToNot(BeNil())
	})

	t.Run("should satisfy Check interface via composition", func(t *testing.T) {
		tc := &mockCheck{
			BaseCheck: check.BaseCheck{
				CheckGroup:       check.GroupComponent,
				Kind:             "test",
				Type:             "type",
				CheckID:          "test.id",
				CheckName:        "Test Name",
				CheckDescription: "Test description",
			},
		}

		var _ check.Check = tc

		g.Expect(tc.ID()).To(Equal("test.id"))
		g.Expect(tc.Name()).To(Equal("Test Name"))
		g.Expect(tc.Description()).To(Equal("Test description"))
		g.Expect(tc.Group()).To(Equal(check.GroupComponent))
	})

	t.Run("should work with different check groups", func(t *testing.T) {
		testCases := []struct {
			name  string
			group check.CheckGroup
		}{
			{"component", check.GroupComponent},
			{"service", check.GroupService},
			{"workload", check.GroupWorkload},
			{"dependency", check.GroupDependency},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				bc := check.BaseCheck{
					CheckGroup:       tc.group,
					Kind:             "test",
					Type:             "test",
					CheckID:          "test.id",
					CheckName:        "Test",
					CheckDescription: "Test",
				}

				g.Expect(bc.Group()).To(Equal(tc.group))
				dr := bc.NewResult()
				g.Expect(dr.Group).To(Equal(string(tc.group)))
			})
		}
	})
}

type mockCheck struct {
	check.BaseCheck
}

func (c *mockCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *mockCheck) Validate(
	_ context.Context,
	_ check.Target,
) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}

func TestBaseCheckIntegration(t *testing.T) {
	g := NewWithT(t)

	t.Run("full check implementation with BaseCheck", func(t *testing.T) {
		mc := &mockCheck{
			BaseCheck: check.BaseCheck{
				CheckGroup:       check.GroupComponent,
				Kind:             "modelmeshserving",
				Type:             check.CheckTypeRemoval,
				CheckID:          "components.modelmesh.removal",
				CheckName:        "Components :: ModelMesh :: Removal (3.x)",
				CheckDescription: "Validates that ModelMesh is disabled",
			},
		}

		g.Expect(mc.ID()).To(Equal("components.modelmesh.removal"))
		g.Expect(mc.Name()).To(Equal("Components :: ModelMesh :: Removal (3.x)"))
		g.Expect(mc.Description()).To(Equal("Validates that ModelMesh is disabled"))
		g.Expect(mc.Group()).To(Equal(check.GroupComponent))

		v2 := semver.MustParse("2.15.0")
		v3 := semver.MustParse("3.0.0")
		target := check.Target{
			CurrentVersion: &v2,
			TargetVersion:  &v3,
		}
		canApply, err := mc.CanApply(t.Context(), target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeTrue())

		dr, err := mc.Validate(t.Context(), check.Target{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Group).To(Equal("component"))
		g.Expect(dr.Kind).To(Equal("modelmeshserving"))
		g.Expect(dr.Name).To(Equal(string(check.CheckTypeRemoval)))
	})
}

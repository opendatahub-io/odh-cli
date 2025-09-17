package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/lburgazzoli/odh-cli/pkg/doctor"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	componentsGV = schema.GroupVersion{Group: "components.platform.opendatahub.io", Version: "v1alpha1"}
	components   = []string{"CodeFlare", "ModelMesh"}
)

func NewComponentsCheck() doctor.DiagnosticCheck {
	return &componentsCheck{}
}

type componentsCheck struct{}

func (c *componentsCheck) Execute(ctx context.Context, cli client.Client) []doctor.Category {
	category := doctor.Category{
		Name:   "Deprecated components",
		Status: doctor.StatusOK,
		Checks: make([]doctor.Check, 0),
	}

	for _, c := range components {
		category.Checks = append(category.Checks, checkDeprecatedComponent(ctx, cli, componentsGV.WithKind(c)))
	}

	// Set the overall status based on checks
	category.Status = doctor.ComputeStatus(category)

	return []doctor.Category{category}
}

func checkDeprecatedComponent(
	ctx context.Context,
	cli client.Client,
	kind schema.GroupVersionKind,
) doctor.Check {
	u := unstructured.Unstructured{}
	u.SetName("default-" + strings.ToLower(kind.Kind))
	u.SetGroupVersionKind(kind)

	err := cli.Get(ctx, client.ObjectKeyFromObject(&u), &u)
	switch {
	case k8serr.IsNotFound(err):
		return doctor.Check{
			Name:    kind.Kind,
			Status:  doctor.StatusOK,
			Message: "Not Enabled",
		}
	case meta.IsNoMatchError(err):
		return doctor.Check{
			Name:    kind.Kind,
			Status:  doctor.StatusOK,
			Message: "Not Enabled",
		}
	case err != nil:
		return doctor.Check{
			Name:    kind.Kind,
			Status:  doctor.StatusError,
			Message: err.Error(),
		}
	default:
		return doctor.Check{
			Name:    kind.Kind,
			Status:  doctor.StatusWarning,
			Message: fmt.Sprintf("%s is deprecated and marked for removal in 3.0", kind.Kind),
		}
	}
}

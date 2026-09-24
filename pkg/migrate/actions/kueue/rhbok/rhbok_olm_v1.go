package rhbok

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	platformcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
)

const rhbokClusterCatalog = "openshift-redhat-operators"

func (a *RHBOKMigrationAction) selectOLMMode(ctx context.Context, target action.Target) error {
	var requested olm.Mode
	switch a.OLMMode {
	case "", "auto":
		if target.Client.ControllerRuntime() != nil {
			found, err := platformcluster.ClusterExtensionInstallsPackage(
				ctx, target.Client.ControllerRuntime(), subscriptionPackage, operatorNamespace,
			)
			if found {
				a.selectedOLMMode = olm.ModeV1

				return nil
			}
			if err != nil && !meta.IsNoMatchError(err) && !apierrors.IsNotFound(err) {
				return fmt.Errorf("check existing RHBOK ClusterExtension: %w", err)
			}
		}
	case string(olm.ModeV0):
		requested = olm.ModeV0
	case string(olm.ModeV1):
		requested = olm.ModeV1
	default:
		return fmt.Errorf("invalid --kueue-olm-mode %q: must be auto, v0, or v1", a.OLMMode)
	}

	mode, err := olm.ResolveInstallMode(target.Client.Discovery(), requested)
	if err != nil {
		return fmt.Errorf("detect RHBOK installation API: %w", err)
	}
	if mode == olm.ModeV1 && target.Client.ControllerRuntime() == nil {
		return errors.New("OLM v1 requires a controller-runtime client")
	}

	a.selectedOLMMode = mode

	return nil
}

func (a *RHBOKMigrationAction) operatorChannel(ctx context.Context, target action.Target) (string, error) {
	if a.selectedOLMMode != olm.ModeV1 {
		return a.resolveSubscriptionChannel(ctx, target)
	}

	channel, found, err := existingClusterExtensionChannel(ctx, target, a.Channel)
	if err != nil {
		return "", err
	}
	if found {
		return channel, nil
	}
	if a.Channel != "" {
		return a.Channel, nil
	}

	return resolveClusterCatalogChannel(ctx, target)
}

func existingClusterExtensionChannel(ctx context.Context, target action.Target, requested string) (string, bool, error) {
	existing, err := findRHBOKClusterExtension(ctx, target.Client.ControllerRuntime())
	if err != nil {
		return "", false, fmt.Errorf("check existing RHBOK ClusterExtension channel: %w", err)
	}
	if existing == nil {
		return "", false, nil
	}

	channels, _, err := unstructured.NestedStringSlice(existing.Object, "spec", "source", "catalog", "channels")
	if err != nil {
		return "", true, fmt.Errorf("read RHBOK ClusterExtension %s channels: %w", existing.GetName(), err)
	}
	if len(channels) == 1 && channels[0] != "" {
		if requested != "" && requested != channels[0] {
			return "", true, fmt.Errorf("--channel %q differs from existing RHBOK ClusterExtension channel %q",
				requested, channels[0])
		}

		return channels[0], true, nil
	}
	if requested != "" {
		return "", true, fmt.Errorf("--channel %q cannot change existing RHBOK ClusterExtension %s",
			requested, existing.GetName())
	}

	return "", true, nil // Existing request has no single channel restriction.
}

func resolveClusterCatalogChannel(ctx context.Context, target action.Target) (string, error) {
	catalog := &unstructured.Unstructured{}
	catalog.SetGroupVersionKind(resources.ClusterCatalog.GVK())
	if err := target.Client.ControllerRuntime().Get(ctx, client.ObjectKey{Name: rhbokClusterCatalog}, catalog); err != nil {
		return "", fmt.Errorf("get RHBOK ClusterCatalog: %w", err)
	}

	baseURL, found, err := unstructured.NestedString(catalog.Object, "status", "urls", "base")
	if err != nil {
		return "", fmt.Errorf("read RHBOK ClusterCatalog content URL: %w", err)
	}
	if !found || baseURL == "" {
		return "", errors.New("RHBOK ClusterCatalog has no content URL at status.urls.base")
	}

	proxy, err := catalogProxyLocation(baseURL)
	if err != nil {
		return "", fmt.Errorf("RHBOK ClusterCatalog content URL: %w", err)
	}
	content, err := target.Client.CoreV1().Services(proxy.namespace).
		ProxyGet(proxy.scheme, proxy.service, proxy.port, proxy.path, nil).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("read RHBOK ClusterCatalog content: %w", err)
	}
	defer func() { _ = content.Close() }()

	channel, err := olm.ResolveCatalogChannel(content, subscriptionPackage)
	if err != nil {
		return "", fmt.Errorf("resolve RHBOK channel from ClusterCatalog: %w", err)
	}

	return channel, nil
}

type catalogProxyTarget struct {
	service   string
	namespace string
	scheme    string
	port      string
	path      string
}

func catalogProxyLocation(baseURL string) (catalogProxyTarget, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return catalogProxyTarget{}, fmt.Errorf("parse URL: %w", err)
	}
	scheme := u.Scheme
	if (scheme != "https" && scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return catalogProxyTarget{}, fmt.Errorf("unsupported catalog URL %q", baseURL)
	}

	hostParts := strings.Split(u.Hostname(), ".")
	if len(hostParts) < 3 || hostParts[0] == "" || hostParts[1] == "" || hostParts[2] != "svc" {
		return catalogProxyTarget{}, fmt.Errorf("catalog URL host %q is not a Kubernetes Service", u.Hostname())
	}

	port := u.Port()
	if port == "" && scheme == "https" {
		port = "443"
	} else if port == "" {
		port = "80"
	}

	return catalogProxyTarget{
		service:   hostParts[0],
		namespace: hostParts[1],
		scheme:    scheme,
		port:      port,
		path:      path.Join("/", u.Path, "api/v1/all"),
	}, nil
}

func (a *RHBOKMigrationAction) serviceAccountName() string {
	if a.ServiceAccount != "" {
		return a.ServiceAccount
	}

	return "odh-cli-dependency-installer"
}

func (a *RHBOKMigrationAction) installRHBOKClusterExtension(
	ctx context.Context,
	target action.Target,
	channel string,
	step action.StepRecorder,
) {
	requested, err := platformcluster.ClusterExtensionInstallsPackage(
		ctx, target.Client.ControllerRuntime(), subscriptionPackage, operatorNamespace,
	)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check RHBOK ClusterExtension: %v", err)

		return
	}

	if target.DryRun {
		if requested {
			step.Completef(result.StepSkipped, "Would use existing RHBOK ClusterExtension")
		} else {
			step.Completef(result.StepSkipped, "Would ensure RHBOK ClusterExtension is installed from channel %s", channel)
		}

		return
	}

	if !requested && channel == "" {
		step.Completef(result.StepFailed, "Existing RHBOK ClusterExtension disappeared before installation; rerun migration")

		return
	}

	if !requested {
		if !target.SkipConfirm {
			target.IO.Fprintln()
			target.IO.Errorf("About to install Red Hat Build of Kueue Operator via OLM v1 (channel: %s)", channel)
			if !confirmation.Prompt(target.IO, "Proceed with operator installation?") {
				step.Completef(result.StepSkipped, "User cancelled installation")

				return
			}
		}

		if err := a.createRHBOKClusterExtension(ctx, target, channel); err != nil {
			step.Completef(result.StepFailed, "Failed to create RHBOK ClusterExtension: %v", err)

			return
		}
	}

	err = wait.PollUntilContextTimeout(ctx, operatorPollPeriod, operatorTimeout, true,
		func(ctx context.Context) (bool, error) {
			info, err := platformcluster.OperatorInstalledViaClusterExtension(
				ctx, target.Client.ControllerRuntime(), subscriptionPackage,
			)
			if err != nil {
				return false, fmt.Errorf("check RHBOK ClusterExtension installation: %w", err)
			}

			return info != nil, nil
		})
	if err != nil {
		step.Completef(result.StepFailed, "Failed waiting for RHBOK ClusterExtension: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Red Hat build of Kueue operator installed and ready via OLM v1")
}

func (a *RHBOKMigrationAction) createRHBOKClusterExtension(
	ctx context.Context,
	target action.Target,
	channel string,
) error {
	account := &corev1.ServiceAccount{}
	err := target.Client.ControllerRuntime().Get(ctx, client.ObjectKey{
		Name: a.serviceAccountName(), Namespace: operatorNamespace,
	}, account)
	if err != nil {
		return fmt.Errorf("OLM v1 requires ServiceAccount %s in namespace %s: %w",
			a.serviceAccountName(), operatorNamespace, err)
	}

	extension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.ClusterExtension.APIVersion(),
		"kind":       resources.ClusterExtension.Kind,
		"metadata":   map[string]any{"name": subscriptionName},
		"spec": map[string]any{
			"namespace":      operatorNamespace,
			"serviceAccount": map[string]any{"name": a.serviceAccountName()},
			"source": map[string]any{
				"sourceType": "Catalog",
				"catalog": map[string]any{
					"packageName": subscriptionPackage,
					"channels":    []any{channel},
					"selector": map[string]any{"matchLabels": map[string]any{
						"olm.operatorframework.io/metadata.name": rhbokClusterCatalog,
					}},
				},
			},
		},
	}}

	err = target.Client.ControllerRuntime().Create(ctx, extension)
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create RHBOK ClusterExtension: %w", err)
	}

	return verifyExistingRHBOKClusterExtension(ctx, target)
}

func verifyExistingRHBOKClusterExtension(ctx context.Context, target action.Target) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(resources.ClusterExtension.GVK())
	if err := target.Client.ControllerRuntime().Get(ctx, client.ObjectKey{Name: subscriptionName}, existing); err != nil {
		return fmt.Errorf("get existing RHBOK ClusterExtension: %w", err)
	}
	packageName, err := jq.Query[string](existing, ".spec.source.catalog.packageName")
	if err != nil {
		return fmt.Errorf("read existing RHBOK ClusterExtension package: %w", err)
	}
	namespace, err := jq.Query[string](existing, ".spec.namespace")
	if err != nil {
		return fmt.Errorf("read existing RHBOK ClusterExtension namespace: %w", err)
	}
	if packageName != subscriptionPackage || namespace != operatorNamespace {
		return fmt.Errorf("ClusterExtension %s already exists for package %q in namespace %q",
			subscriptionName, packageName, namespace)
	}

	return nil
}

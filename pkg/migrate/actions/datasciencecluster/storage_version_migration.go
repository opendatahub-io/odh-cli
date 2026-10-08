package datasciencecluster

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	requestName = "dsc-storage-migration"

	// storageVersionAnnotation records on a request the storage version it
	// was created to migrate to, so a later run can tell whether an existing
	// request is its own.
	storageVersionAnnotation = "odh-cli.opendatahub.io/storage-version"

	pollInterval = time.Second

	msgNoMigrationAPI = "no supported StorageVersionMigration API is available; " +
		"enable the cluster storage migration API and controller before retrying"

	msgStaleRequest = "existing migration request %s is for storage version %q, not %q; " +
		"review it, delete it, and retry"

	msgStorageChanged = "DataScienceCluster storage version changed during migration; rerun on a stable cluster"
)

// storageVersionMigration is one run of the storage migration: it asks the cluster's storage
// migration controller to rewrite every DataScienceCluster at the storage
// version, then removes the older versions from the CRD's storedVersions.
type storageVersionMigration struct {
	target  action.Target
	step    action.StepRecorder
	storage string
}

func (m *storageVersionMigration) run(ctx context.Context) error {
	api, err := m.servedAPI()

	switch {
	case err != nil:
		return err
	case api == nil:
		return errors.New(msgNoMigrationAPI)
	}

	requests := m.target.Client.Dynamic().Resource(api.GVR())

	_, err = requests.Create(ctx, m.newRequest(*api), metav1.CreateOptions{})

	switch {
	// A request left by an interrupted run is picked up rather than recreated.
	case apierrors.IsAlreadyExists(err):
		err = m.checkExistingRequest(ctx, requests)
		if err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("creating %s migration request: %w", api.APIVersion(), err)
	}

	m.step.Recordf("migration-request", "Using %s %s", result.StepCompleted, api.APIVersion(), requestName)
	m.step.AddDetail("migrationRequest", requestName)

	err = m.waitForRequest(ctx, requests)
	if err != nil {
		return fmt.Errorf("waiting for migration request %s (retained for diagnosis): %w", requestName, err)
	}

	err = m.pruneStoredVersions(ctx)
	if err != nil {
		return fmt.Errorf("pruning storage history (migration request %q retained): %w", requestName, err)
	}

	err = requests.Delete(ctx, requestName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deleting completed migration request %s: %w", requestName, err)
	}

	return nil
}

// servedAPI returns the first StorageVersionMigration API the cluster serves,
// or nil when it serves none. Newer Kubernetes APIs are preferred and the
// OpenShift migrator comes last.
func (m *storageVersionMigration) servedAPI() (*resources.ResourceType, error) {
	candidates := []resources.ResourceType{
		resources.StorageVersionMigration,
		resources.StorageVersionMigrationBeta,
		resources.StorageVersionMigrationAlpha,
		resources.OpenShiftStorageVersionMigration,
	}

	for _, candidate := range candidates {
		served, err := m.target.Client.Discovery().ServerResourcesForGroupVersion(candidate.APIVersion())

		switch {
		case apierrors.IsNotFound(err):
			continue
		case err != nil:
			return nil, fmt.Errorf("discovering storage migration APIs: %w", err)
		}

		for _, r := range served.APIResources {
			if r.Name == candidate.Resource {
				return &candidate, nil
			}
		}
	}

	return nil, nil
}

func (m *storageVersionMigration) newRequest(api resources.ResourceType) *unstructured.Unstructured {
	resource := map[string]any{
		"group":    resources.DataScienceCluster.Group,
		"resource": resources.DataScienceCluster.Resource,
	}

	// Both alpha APIs require a GVR; Kubernetes v1 and v1beta1 use a GroupResource.
	if api.Version == "v1alpha1" {
		resource["version"] = m.storage
	}

	request := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"resource": resource,
			},
		},
	}

	request.SetGroupVersionKind(api.GVK())
	request.SetName(requestName)
	request.SetAnnotations(map[string]string{
		storageVersionAnnotation: m.storage,
	})

	return request
}

// checkExistingRequest accepts an existing request only if it was created for
// the current storage version. A request for an earlier storage version may
// already have succeeded, and waiting on it would prune storedVersions without
// the objects ever being rewritten at the current one.
func (m *storageVersionMigration) checkExistingRequest(
	ctx context.Context,
	requests dynamic.ResourceInterface,
) error {
	existing, err := requests.Get(ctx, requestName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading existing migration request %s: %w", requestName, err)
	}

	existingStorage := existing.GetAnnotations()[storageVersionAnnotation]
	if existingStorage != m.storage {
		return fmt.Errorf(msgStaleRequest, requestName, existingStorage, m.storage)
	}

	return nil
}

// waitForRequest blocks until the controller marks the request succeeded or
// failed, or the context ends.
func (m *storageVersionMigration) waitForRequest(ctx context.Context, requests dynamic.ResourceInterface) error {
	condition := func(ctx context.Context) (bool, error) {
		request, err := requests.Get(ctx, requestName, metav1.GetOptions{})
		if err != nil {
			return false, fmt.Errorf("reading migration request: %w", err)
		}

		conditions, err := jq.Query[[]metav1.Condition](request, ".status.conditions // []")
		if err != nil {
			return false, fmt.Errorf("reading migration conditions: %w", err)
		}

		succeeded := false

		for _, c := range conditions {
			if c.Status != metav1.ConditionTrue {
				continue
			}

			switch c.Type {
			// Failure wins if a malformed controller reports both outcomes.
			case "Failed":
				return false, fmt.Errorf("migration failed: %s: %s", c.Reason, c.Message)
			case "Succeeded":
				succeeded = true
			}
		}

		return succeeded, nil
	}

	//nolint:wrapcheck // The caller adds the request name.
	return wait.PollUntilContextCancel(ctx, pollInterval, true, condition)
}

// pruneStoredVersions leaves only the storage version in the CRD's
// storedVersions. It re-reads the CRD so the update carries a fresh
// resourceVersion and fails on a concurrent change.
func (m *storageVersionMigration) pruneStoredVersions(ctx context.Context) error {
	crd, current, err := readStorage(ctx, m.target)

	switch {
	case err != nil:
		return err
	case current != m.storage:
		return errors.New(msgStorageChanged)
	case slices.Equal(crd.Status.StoredVersions, []string{m.storage}):
		return nil
	}

	crd.Status.StoredVersions = []string{m.storage}

	definitions := m.target.Client.APIExtensions().ApiextensionsV1().CustomResourceDefinitions()

	_, err = definitions.UpdateStatus(ctx, crd, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating DataScienceCluster CRD storedVersions: %w", err)
	}

	return nil
}

package dspa

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

const (
	trustedCABundleName = "trusted-ca-bundle"
	mariaDBPVCPrefix    = "mariadb-"
	minioPVCPrefix      = "minio-"
	//nolint:gosec // False positive - JQ path string, not hardcoded credentials
	pathExternalS3Creds = ".spec.objectStorage.externalStorage.s3CredentialsSecret.secretName"
	//nolint:gosec // False positive - JQ path string, not hardcoded credentials
	pathMinioS3Creds = ".spec.objectStorage.minio.s3CredentialsSecret.secretName"
	//nolint:gosec // False positive - JQ path string, not hardcoded credentials
	pathMariaDBPassword = ".spec.database.mariaDB.passwordSecret.name"
	//nolint:gosec // False positive - JQ path string, not hardcoded credentials
	pathExternalDBPassword = ".spec.database.externalDB.passwordSecret.name"
	pathCABundle           = ".spec.apiServer.cABundle.configMapName"
	pathCustomServerConfig = ".spec.apiServer.customServerConfigMap.name"
	pathCustomKFPLauncher  = ".spec.apiServer.customKfpLauncherConfigMap"
	pathDeployMariaDB      = ".spec.database.mariaDB.deploy"
	pathDeployMinio        = ".spec.objectStorage.minio.deploy"
)

// Resolver resolves dependencies for DataSciencePipelinesApplication (DSPA) workloads.
type Resolver struct{}

// NewResolver creates a new DSPA dependency resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// CanHandle returns true for DataSciencePipelinesApplication resources (both v1 and v1alpha1).
func (r *Resolver) CanHandle(gvr schema.GroupVersionResource) bool {
	return gvr.Group == resources.DataSciencePipelinesApplicationV1.Group &&
		gvr.Resource == resources.DataSciencePipelinesApplicationV1.Resource
}

// Resolve finds all dependencies for a DataSciencePipelinesApplication.
func (r *Resolver) Resolve(
	ctx context.Context,
	c client.Reader,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	namespace := obj.GetNamespace()
	dspaName := obj.GetName()
	var deps []dependencies.Dependency

	// Resolve Secrets
	secretDeps, err := r.resolveSecrets(ctx, c, namespace, obj)
	if err != nil {
		return nil, err
	}
	deps = append(deps, secretDeps...)

	// Resolve ConfigMaps
	configMapDeps, err := r.resolveConfigMaps(ctx, c, namespace, obj)
	if err != nil {
		return nil, err
	}
	deps = append(deps, configMapDeps...)

	// Resolve PVCs
	pvcDeps, err := r.resolvePVCs(ctx, c, namespace, dspaName, obj)
	if err != nil {
		return nil, err
	}
	deps = append(deps, pvcDeps...)

	return deps, nil
}

func (r *Resolver) resolveSecrets(
	ctx context.Context,
	c client.Reader,
	namespace string,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	var secretNames []string

	// External storage S3 credentials
	if name := r.queryStringField(obj, pathExternalS3Creds); name != "" {
		secretNames = append(secretNames, name)
	}

	// Minio S3 credentials
	if name := r.queryStringField(obj, pathMinioS3Creds); name != "" {
		secretNames = append(secretNames, name)
	}

	// MariaDB password secret
	if name := r.queryStringField(obj, pathMariaDBPassword); name != "" {
		secretNames = append(secretNames, name)
	}

	// ExternalDB password secret
	if name := r.queryStringField(obj, pathExternalDBPassword); name != "" {
		secretNames = append(secretNames, name)
	}

	if len(secretNames) == 0 {
		return nil, nil
	}

	items, errors, err := kube.FetchResourcesByNameWithErrors(
		ctx,
		c,
		namespace,
		resources.Secret,
		secretNames,
	)
	if err != nil {
		return nil, fmt.Errorf("fetching Secrets: %w", err)
	}

	deps := make([]dependencies.Dependency, 0, len(secretNames))

	// Add found resources
	for _, res := range items {
		deps = append(deps, dependencies.Dependency{
			GVR:      resources.Secret.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	// Add missing resources with error info
	for name, fetchErr := range errors {
		deps = append(deps, dependencies.Dependency{
			GVR:      resources.Secret.GVR(),
			Resource: nil,
			Name:     name,
			Error:    fetchErr,
		})
	}

	return deps, nil
}

func (r *Resolver) resolveConfigMaps(
	ctx context.Context,
	c client.Reader,
	namespace string,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	var configMapNames []string

	// CA Bundle ConfigMap
	if name := r.queryStringField(obj, pathCABundle); name != "" && name != trustedCABundleName {
		configMapNames = append(configMapNames, name)
	}

	// Custom server config ConfigMap
	if name := r.queryStringField(obj, pathCustomServerConfig); name != "" {
		configMapNames = append(configMapNames, name)
	}

	// Custom KFP launcher ConfigMap
	if name := r.queryStringField(obj, pathCustomKFPLauncher); name != "" {
		configMapNames = append(configMapNames, name)
	}

	if len(configMapNames) == 0 {
		return nil, nil
	}

	items, err := kube.FetchResourcesByName(ctx, c, namespace, resources.ConfigMap, configMapNames)
	if err != nil {
		return nil, fmt.Errorf("fetching ConfigMaps: %w", err)
	}

	deps := make([]dependencies.Dependency, 0, len(items))
	for _, res := range items {
		deps = append(deps, dependencies.Dependency{
			GVR:      resources.ConfigMap.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	return deps, nil
}

func (r *Resolver) resolvePVCs(
	ctx context.Context,
	c client.Reader,
	namespace string,
	dspaName string,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	var pvcNames []string

	// MariaDB PVC (operator-created when database deployment enabled)
	deployMariaDB, err := jq.Query[bool](obj, pathDeployMariaDB)
	if err == nil && deployMariaDB {
		pvcNames = append(pvcNames, mariaDBPVCPrefix+dspaName)
	}

	// Minio PVC (operator-created when Minio deployment enabled)
	deployMinio, err := jq.Query[bool](obj, pathDeployMinio)
	if err == nil && deployMinio {
		pvcNames = append(pvcNames, minioPVCPrefix+dspaName)
	}

	if len(pvcNames) == 0 {
		return nil, nil
	}

	items, err := kube.FetchResourcesByName(ctx, c, namespace, resources.PersistentVolumeClaim, pvcNames)
	if err != nil {
		return nil, fmt.Errorf("fetching PVCs: %w", err)
	}

	deps := make([]dependencies.Dependency, 0, len(items))
	for _, res := range items {
		deps = append(deps, dependencies.Dependency{
			GVR:      resources.PersistentVolumeClaim.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	return deps, nil
}

// queryStringField queries a string field from the object, returning empty string if not found or on error.
func (r *Resolver) queryStringField(obj *unstructured.Unstructured, path string) string {
	value, err := jq.Query[string](obj, path)
	if err != nil {
		return ""
	}

	return value
}

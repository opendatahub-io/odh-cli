package rhbok

import (
	"context"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

//nolint:gochecknoglobals // export_test.go exposes internals for external test package
var (
	ExportCheckCurrentKueueState = (*RHBOKMigrationAction).checkCurrentKueueState
	ExportCheckNoRHBOKConflicts  = (*RHBOKMigrationAction).checkNoRHBOKConflicts
	ExportVerifyKueueResources   = (*RHBOKMigrationAction).verifyKueueResources
	ExportCheckKueueManaged      = (*RHBOKMigrationAction).checkKueueManaged
	ExportCheckCertManager       = (*RHBOKMigrationAction).checkCertManager
	ExportCheckOperatorChannel   = (*RHBOKMigrationAction).checkOperatorChannel
	ExportReportLabelingPlan     = (*RHBOKMigrationAction).reportLabelingPlan
	ExportPreserveKueueConfig    = (*RHBOKMigrationAction).preserveKueueConfig
	ExportRemoveEmbeddedKueue    = (*RHBOKMigrationAction).removeEmbeddedKueue
	ExportActivateRHBOK          = (*RHBOKMigrationAction).activateRHBOK
	ExportInstallRHBOKOperator   = (*RHBOKMigrationAction).installRHBOKOperator
	ExportDeleteLegacyCRDs       = (*RHBOKMigrationAction).deleteLegacyCRDs
	ExportLabelKueueNamespaces   = (*RHBOKMigrationAction).labelKueueNamespaces
	ExportLabelKueueWorkloads    = (*RHBOKMigrationAction).labelKueueWorkloads
	ExportVerifyMigration        = (*RHBOKMigrationAction).verifyMigrationComplete
	ExportVerifyResources        = (*RHBOKMigrationAction).verifyResourcesPreserved
	ExportWaitEmbeddedRemoval    = (*RHBOKMigrationAction).waitForEmbeddedRemoval
	ExportWaitKueueReady         = (*RHBOKMigrationAction).waitForKueueReady
	ExportDiscoverLabelingPlan   = (*RHBOKMigrationAction).discoverLabelingPlan
	ExportPreparePermissions     = preparePermissions
	ExportRunPermissions         = runPermissions

	ExportConfigMapName         = configMapName
	ExportApplicationsNamespace = applicationsNamespace
	ExportOperatorNamespace     = operatorNamespace
	ExportSubscriptionName      = subscriptionName
	ExportCSVNamePrefix         = csvNamePrefix
	ExportEmbeddedDeployment    = embeddedKueueDeployment

	ExportIsMigrationComplete = (*RHBOKMigrationAction).isMigrationComplete
)

func ExportPlanNamespaces(p labelingPlan) []string {
	return p.namespaces
}

func ExportPlanWorkloads(p labelingPlan) []workloadRef {
	return p.workloads
}

func ExportWorkloadQueueName(w workloadRef) string {
	return w.queueName
}

func ExportVerifyRBAC(a *RHBOKMigrationAction, ctx context.Context, target action.Target, checks []rbac.PermissionCheck) {
	a.verifyRBAC(ctx, target, checks)
}

func SetTestPollConfig(period, timeout time.Duration) {
	testComponentPollPeriod = period
	testComponentTimeout = timeout
}

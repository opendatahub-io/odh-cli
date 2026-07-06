package migrate

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/llamastack/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	rayactions "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/raycluster"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/training"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/data"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/deadlock"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/guardrails"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/metrics"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/trustyai/otelexporter"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/authmodel"
	workbenchcleanup "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/cleanup"
	workbenchkueue "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/upgrade"
	workbenchverify "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/verify"
)

func newDefaultRegistry() *action.ActionRegistry {
	registry := action.NewActionRegistry()

	registry.MustRegister(&rhbok.RHBOKMigrationAction{})
	registry.MustRegister(&aipipelines.PreUpgradeCheckAction{})
	registry.MustRegister(&aipipelines.UpdateDSPRoleAction{})
	registry.MustRegister(&aipipelines.PostUpgradeCheckAction{})
	registry.MustRegister(&modelserving.ServerlessToRawAction{})
	registry.MustRegister(&modelserving.ModelMeshToRawAction{})
	registry.MustRegister(&modelserving.HardwareProfilesIgnorelistAction{})
	registry.MustRegister(&modelserving.AddOwnerReferencesAction{})
	registry.MustRegister(&modelserving.ManagedISVCConfigAction{})
	registry.MustRegister(&upgrade.WorkbenchUpgradeAction{})
	wbScope := &workbenches.SharedScopeOptions{}
	cleanupAction := &workbenchcleanup.CleanupOAuthAction{Scope: wbScope}
	registry.MustRegister(&workbenchkueue.AttachKueueLabelAction{Scope: wbScope})
	registry.MustRegister(cleanupAction)
	registry.MustRegister(&authmodel.PatchAuthModelAction{Scope: wbScope, CleanupAction: cleanupAction})
	registry.MustRegister(&workbenchverify.VerifyMigrationAction{Scope: wbScope})
	registry.MustRegister(&deadlock.BreakGPUDeadlockAction{})
	registry.MustRegister(&guardrails.PatchGuardrailsAction{})
	registry.MustRegister(&otelexporter.MigrateOtelExporterAction{})
	registry.MustRegister(&metrics.MetricsAction{})
	registry.MustRegister(&data.DataAction{})
	registry.MustRegister(&training.VerifyWorkloadsAction{})
	registry.MustRegister(&backup.LlamaStackBackupAction{})
	rayBackup, rayMigrate := rayactions.NewActions()
	registry.MustRegister(rayBackup)
	registry.MustRegister(rayMigrate)

	return registry
}

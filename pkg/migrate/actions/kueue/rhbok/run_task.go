package rhbok

import (
	"context"
	"errors"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
)

type runTask struct {
	action *RHBOKMigrationAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.checkCurrentKueueState(ctx, target)
	t.action.checkNoRHBOKConflicts(ctx, target)
	t.action.verifyKueueResources(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	kueueManaged := t.action.checkKueueManaged(ctx, target)

	if kueueManaged {
		t.action.preserveKueueConfig(ctx, target)
	}

	t.action.installRHBOKOperator(ctx, target)
	t.action.updateDataScienceCluster(ctx, target)
	t.action.verifyResourcesPreserved(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

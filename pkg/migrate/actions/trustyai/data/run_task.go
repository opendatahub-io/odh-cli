package data

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	podexec "github.com/opendatahub-io/odh-cli/pkg/util/exec"
)

type runTask struct {
	action *DataAction
}

func (t *runTask) Validate(_ context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-data-restore", "Check data restore readiness")

	if t.action.BackupFile == "" {
		step.Completef(result.StepSkipped, "No --data-file specified; nothing to restore")

		return action.BuildResult(target)
	}

	info, err := os.Stat(t.action.BackupFile)
	if err != nil {
		step.Completef(result.StepFailed, "Backup path %s not found: %v", t.action.BackupFile, err)

		return action.BuildResult(target)
	}

	if !info.IsDir() {
		step.Completef(result.StepFailed, "Backup path %s is not a directory", t.action.BackupFile)

		return action.BuildResult(target)
	}

	storageFormat, err := detectBackupType(t.action.BackupFile)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect backup type: %v", err)

		return action.BuildResult(target)
	}

	step.Completef(result.StepCompleted, "Ready to restore %s backup from %s", storageFormat, t.action.BackupFile)

	return action.BuildResult(target)
}

func (t *runTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("restore-trustyai-data", "Restore TrustyAI data storage")

	if t.action.BackupFile == "" {
		step.Completef(result.StepSkipped, "No --data-file specified; nothing to restore")

		return action.BuildResult(target)
	}

	storageFormat, err := detectBackupType(t.action.BackupFile)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect backup type: %v", err)

		return action.BuildResult(target)
	}

	var executor podexec.Executor
	if target.RESTConfig != nil {
		executor = podexec.NewSPDYExecutor(target.RESTConfig, target.Client.CoreV1())
	}

	meta := loadMetadata(t.action.BackupFile)

	namespace := ""
	if meta != nil {
		namespace = meta.Namespace
	}

	targetSvc, err := resolveTargetService(ctx, target, t.action.ServiceName, meta, namespace)
	if err != nil {
		step.Completef(result.StepFailed, "%v", err)

		return action.BuildResult(target)
	}

	switch storageFormat {
	case storageFormatPVC:
		t.restorePVC(ctx, target, executor, *targetSvc, meta, step)
	case storageFormatDatabase:
		t.restoreDatabase(ctx, target, executor, *targetSvc, meta, step)
	default:
		step.Completef(result.StepFailed, "Unsupported backup type %q", storageFormat)
	}

	return action.BuildResult(target)
}

func (t *runTask) restorePVC(
	ctx context.Context,
	target action.Target,
	executor podexec.Executor,
	svc serviceInfo,
	meta *DataBackupMetadata,
	step action.StepRecorder,
) {
	dataDir := filepath.Join(t.action.BackupFile, dataSubDir)

	fileCount, err := countFiles(dataDir)
	if err != nil || fileCount == 0 {
		step.Completef(result.StepSkipped, "Backup directory is empty, nothing to restore")

		return
	}

	pod, err := findServicePod(ctx, target, svc.namespace, svc.name)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find TrustyAI pod: %v", err)

		return
	}

	var mount *mountInfo

	if meta != nil && meta.MountPath != "" {
		mount = &mountInfo{mountPath: meta.MountPath, pvcName: meta.PVCName}
	} else {
		mount, err = findMountPath(pod, svc.cr, svc.name)
		if err != nil {
			step.Completef(result.StepFailed, "Failed to find mount path: %v", err)

			return
		}
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would restore %d file(s) to %s:%s", fileCount, pod.Name, mount.mountPath)

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to restore %d file(s) to pod %s at %s", fileCount, pod.Name, mount.mountPath)

		if !confirmation.Prompt(target.IO, "Proceed with PVC restore?") {
			step.Completef(result.StepSkipped, "User cancelled PVC restore")

			return
		}
	}

	if executor == nil {
		step.Completef(result.StepFailed, "No SPDY executor available (RESTConfig not set)")

		return
	}

	if err := podexec.CopyToPod(ctx, executor, podexec.CopyOptions{
		Namespace:     svc.namespace,
		PodName:       pod.Name,
		ContainerName: trustyaiContainer,
		PodPath:       mount.mountPath,
		LocalPath:     dataDir,
	}); err != nil {
		step.Completef(result.StepFailed, "Failed to copy data to pod: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Restored %d file(s) to %s:%s", fileCount, pod.Name, mount.mountPath)
}

func (t *runTask) restoreDatabase(
	ctx context.Context,
	target action.Target,
	executor podexec.Executor,
	svc serviceInfo,
	meta *DataBackupMetadata,
	step action.StepRecorder,
) {
	dumpPath := filepath.Join(t.action.BackupFile, dumpFileName)

	dumpLines, err := countLines(dumpPath)
	if err != nil || dumpLines <= 1 {
		step.Completef(result.StepFailed, "Dump file appears empty or unreadable")

		return
	}

	creds, err := findDBCredentials(ctx, target, svc.namespace, svc.cr, svc.name)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find DB credentials: %v", err)

		return
	}

	secretObj, err := target.Client.GetResource(ctx, resources.Secret, creds.secretName,
		client.InNamespace(svc.namespace))
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get secret: %v", err)

		return
	}

	secretData := extractSecretData(secretObj)

	mariadbPod, err := t.resolveMariaDBPod(ctx, target, svc, meta, secretData)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to find MariaDB pod: %v", err)

		return
	}

	// Check dry-run before requiring a live SPDY executor or detecting the
	// remote client command: a dry-run only needs to report what would
	// happen and must not depend on RESTConfig/exec being available.
	if target.DryRun {
		step.Completef(result.StepSkipped, "Would restore %d-line dump to database %s via %s",
			dumpLines, creds.database, mariadbPod.Name)

		return
	}

	if executor == nil {
		step.Completef(result.StepFailed, "No SPDY executor available (RESTConfig not set)")

		return
	}

	clientCmd, err := detectClientCommand(ctx, executor, svc.namespace, mariadbPod)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to detect client command: %v", err)

		return
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("About to restore %d-line SQL dump to database %s via %s",
			dumpLines, creds.database, mariadbPod.Name)

		if !confirmation.Prompt(target.IO, "Proceed with database restore?") {
			step.Completef(result.StepSkipped, "User cancelled database restore")

			return
		}
	}

	dumpFile, err := os.Open(dumpPath) //nolint:gosec // Path from validated backup dir.
	if err != nil {
		step.Completef(result.StepFailed, "Failed to open dump file: %v", err)

		return
	}
	defer func() { _ = dumpFile.Close() }()

	var stderrBuf bytes.Buffer

	execErr := executor.Exec(ctx, podexec.ExecOptions{
		Namespace:     svc.namespace,
		PodName:       mariadbPod.Name,
		ContainerName: mariadbContainer,
		Command: []string{
			clientCmd,
			"-u" + creds.username,
			"-p" + creds.password,
			creds.database,
		},
		Stdin:  dumpFile,
		Stderr: &stderrBuf,
	})

	if execErr != nil {
		if containsSQLError(stderrBuf.String()) {
			step.Completef(result.StepFailed, "SQL error during restore: %s", stderrBuf.String())

			return
		}

		target.IO.Errorf("WARNING: restore completed with warnings: %s", stderrBuf.String())
	}

	step.Completef(result.StepCompleted, "Restored %d-line dump to database %s via %s",
		dumpLines, creds.database, mariadbPod.Name)
}

func (t *runTask) resolveMariaDBPod(
	ctx context.Context,
	target action.Target,
	svc serviceInfo,
	meta *DataBackupMetadata,
	secretData map[string][]byte,
) (*corev1.Pod, error) {
	if meta != nil && meta.MariaDBPod != "" {
		pod, _ := findRunningPodByName(ctx, target, svc.namespace, meta.MariaDBPod)
		if pod != nil {
			return pod, nil
		}
	}

	return findMariaDBPod(ctx, target, svc.namespace, svc.name, secretData)
}

func resolveTargetService(
	ctx context.Context,
	target action.Target,
	serviceName string,
	meta *DataBackupMetadata,
	namespace string,
) (*serviceInfo, error) {
	services, err := discoverTrustyAIServices(ctx, target, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to discover TrustyAI services: %w", err)
	}

	if svc := matchServiceByMeta(services, meta, namespace); svc != nil {
		return svc, nil
	}

	if svc := matchServiceByNamespace(services, namespace); svc != nil {
		return svc, nil
	}

	if len(services) > 0 {
		return &services[0], nil
	}

	return nil, errors.New("no TrustyAIService found for restore")
}

func matchServiceByMeta(services []serviceInfo, meta *DataBackupMetadata, namespace string) *serviceInfo {
	if meta == nil || meta.TrustyAIService == "" {
		return nil
	}

	for i := range services {
		if services[i].name != meta.TrustyAIService {
			continue
		}

		if namespace != "" && services[i].namespace != namespace {
			continue
		}

		return &services[i]
	}

	return nil
}

func matchServiceByNamespace(services []serviceInfo, namespace string) *serviceInfo {
	if namespace == "" {
		return nil
	}

	for i := range services {
		if services[i].namespace == namespace {
			return &services[i]
		}
	}

	return nil
}

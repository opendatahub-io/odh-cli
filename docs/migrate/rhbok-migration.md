# RHBOK Kueue Migration (As-Built)

**Scope:** This document describes the implemented `kueue.rhbok.migrate` action.

It migrates OpenShift AI **built-in (embedded) Kueue** to the **Red Hat build of Kueue (RHBOK)** operator during the **pre-upgrade** phase.

For operator-facing CLI examples, see [upgrade-helpers-comparison.md](upgrade-helpers-comparison.md#kueue). For general CLI design, see [../design.md](../design.md).

## Overview

| Field | Value |
|-------|-------|
| Action ID | `kueue.rhbok.migrate` |
| Group | `migration` |
| Phase | `pre-upgrade` |
| Package | `pkg/migrate/actions/kueue/rhbok` |
| Registration | Explicit `MustRegister` in `pkg/migrate/registry.go` |

Goal: hand off from DSC-managed embedded Kueue (`managementState=Managed`) to RHBOK (`managementState=Unmanaged`), install the OLM operator, remove incompatible legacy CRDs, label namespaces/workloads for RHBOK, and verify the handoff.

There is **no separate cleanup task**. Embedded removal and legacy CRD deletion run as part of `migrate run`.

## When It Applies

`CanApply` returns true only when `CurrentVersion` is set and:

```text
Major == 2 && Minor >= 25
```

`TargetVersion` is **not** consulted. Clusters already on 3.x current version do not match this gate.

## CLI Surface

```bash
# Preflight + backup (when Managed)
kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version <X.Y.Z>

# Execute migration
kubectl odh migrate run --migration kueue.rhbok.migrate --target-version <X.Y.Z>
```

Shared command flags (`--dry-run`, `--yes`, `--output-dir`, etc.) come from the prepare/run commands. Action-specific flags from `AddFlags`:

| Flag | Default | Purpose |
|------|---------|---------|
| `--cluster-queue-name` | `""` | Optional DSC `defaultClusterQueueName` |
| `--local-queue-name` | `""` | Optional DSC `defaultLocalQueueName` |
| `--workload-queue-name` | `default` | Value for `kueue.x-k8s.io/queue-name` on workloads |
| `--channel` | catalog-resolved | OLM channel; fallback `stable-v1.2` |
| `--skip-remove-embedded` | `false` | Skip Managed → Removed (not recommended) |
| `--force-delete-legacy-crds` | `false` | Delete legacy CRDs even when instances exist |

## Package Map

| File | Responsibility |
|------|----------------|
| `rhbok.go` | Action metadata, flags, `CanApply`, ConfigMap preservation, OLM install helpers |
| `prepare_task.go` | `migrate prepare` Validate + Execute (preflight + backup) |
| `run_task.go` | `migrate run` Validate + Execute (orchestration, halt-on-failure) |
| `rhbok_preflight.go` | RBAC, cert-manager, Kueue state, RHBOK conflicts, channel, inventory |
| `rhbok_dsc.go` | DSC mutations: Removed / Unmanaged |
| `rhbok_crds.go` | Delete legacy v1alpha1 Cohort/Topology CRDs |
| `rhbok_wait.go` | Wait for embedded removal and KueueReady + pods |
| `rhbok_labeling.go` | Discover, report, and apply namespace/workload labels |
| `rhbok_verify.go` | Post-migration verification and queue count check |

## Framework Wiring

The action implements `action.Action` and `action.ActionConfigurer`:

- `Prepare() → prepareTask`
- `Run() → runTask`
- Each task implements `Validate` then `Execute` (`Execute` re-runs `Validate` first)

Fail-fast: if any validation step fails, Execute returns without mutating. During run, `haltIfStepFailed` stops the chain after a failed named step (including failed children).

## Prepare Flow

### Validate

1. RBAC (`preparePermissions`)
2. cert-manager present (`certificates.cert-manager.io` CRD, or `cert-manager` / `openshift-cert-manager` namespace)
3. Current Kueue `managementState` on DataScienceCluster
4. Inventory ClusterQueues / LocalQueues
5. Report labeling plan (namespaces + workloads that would be labeled)

### Execute

If Kueue is **Managed**:

1. Backup ClusterQueues (YAML under `--output-dir`)
2. Backup ConfigMap `kueue-manager-config` in `redhat-ods-applications`

If not Managed: step `backup-skipped`.

Prepare does **not** backup LocalQueues or the DataScienceCluster.

## Run Flow

### Validate (stricter than prepare)

All prepare checks, plus:

- RBAC with run permissions (update/create/delete/patch)
- Fail if Managed **and** an existing RHBOK subscription is already present (`checkNoRHBOKConflicts`)
- Resolve/check OLM channel

### Execute

If already complete (Unmanaged + CSV Succeeded + kueue pods ready): single skipped step `migration-complete`.

Otherwise `executeMigration` (Managed path shown):

```text
preserve-kueue-config       Annotate kueue-manager-config opendatahub.io/managed=false
remove-embedded-kueue       DSC managementState → Removed  (unless --skip-remove-embedded)
wait-embedded-removal       Wait until Removed / deployment gone
delete-legacy-crds          Delete cohorts.kueue.x-k8s.io, topologies.kueue.x-k8s.io
install-rhbok-operator      OLM Subscription kueue-operator in openshift-kueue-operator
activate-rhbok              DSC managementState → Unmanaged (+ optional queue name fields)
wait-kueue-ready            KueueReady=True + operator namespace pods ready
label-kueue-namespaces      kueue.openshift.io/managed=true
label-kueue-workloads       kueue.x-k8s.io/queue-name=<value>
verify-migration-complete   Multi-check verification
verify-resources-preserved  Recount ClusterQueues / LocalQueues
```

State machine for the DSC kueue component:

```text
Managed → Removed → Unmanaged
```

## Resources Touched

| Resource | Detail |
|----------|--------|
| DataScienceCluster | `.spec.components.kueue.managementState`; optional default queue name fields |
| Embedded deployment | `kueue-controller-manager` in `redhat-ods-applications` |
| ConfigMap | `kueue-manager-config` in `redhat-ods-applications` |
| OLM | Package/Subscription `kueue-operator`, source `redhat-operators`, NS `openshift-kueue-operator` |
| Legacy CRDs | `cohorts.kueue.x-k8s.io`, `topologies.kueue.x-k8s.io` (v1alpha1) |
| Live queues | `clusterqueues.kueue.x-k8s.io`, `localqueues.kueue.x-k8s.io` (v1beta1) |
| Namespace label | `kueue.openshift.io/managed=true` |
| Workload label | `kueue.x-k8s.io/queue-name` |

Workload kinds labeled/verified (from `pkg/lint/checks/kueue/discovery`): Notebook, InferenceService, LLMInferenceService, RayCluster, RayJob, PyTorchJob.

## Known Limitations

- Applications namespace is hardcoded to `redhat-ods-applications` (RHOAI), not ODH `opendatahub`.
- Queue “preservation” verification compares **counts**, not backup diffs.
- Prepare does not backup LocalQueues or DSC.
- `waitForKueueReady` lists all pods in the operator namespace; some verify/complete checks filter `app.kubernetes.io/name=kueue`.

## Related

- Code: `pkg/migrate/actions/kueue/rhbok`
- Operator UX: [upgrade-helpers-comparison.md](upgrade-helpers-comparison.md#kueue)
- Shared discovery: `pkg/lint/checks/kueue/discovery`
- Historical greenfield plan (superseded): [implementation-plan.md](implementation-plan.md)

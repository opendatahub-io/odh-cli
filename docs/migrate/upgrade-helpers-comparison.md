# Migrating from rhoai-upgrade-helpers to rhai-cli

This document compares the deprecated [rhoai-upgrade-helpers](https://github.com/red-hat-data-services/rhoai-upgrade-helpers) bash/Python scripts with their equivalent `rhai-cli migrate` commands. The CLI consolidates all upgrade operations into a single binary with consistent flags, dry-run support, and structured output.

> **Note:** The `rhoai-upgrade-helpers` repository is deprecated and no longer maintained.
> **Naming:** `rhai-cli` is the binary name used in this doc's examples -- it's the entrypoint inside the container image (`/opt/rhai-cli/bin/rhai-cli`). If you install the tool as a kubectl plugin instead, invoke it as `kubectl odh` (e.g. `kubectl odh migrate list ...`).

---

## Table of Contents

- [Quick Reference](#quick-reference)
- [Getting Started](#getting-started)
- [Key Differences](#key-differences)
- [Workbenches](#workbenches)
- [RayCluster](#raycluster)
- [Model Serving](#model-serving)
- [AI Pipelines](#ai-pipelines)
- [TrustyAI](#trustyai)
- [Kubeflow Training](#kubeflow-training)
- [LlamaStack](#llamastack)
- [Kueue](#kueue)
- [End-to-End Upgrade Workflow](#end-to-end-upgrade-workflow)

---

## Quick Reference

| Component                    | Bash Script(s)                                       | rhai-cli Action ID(s)                      |
|------------------------------|------------------------------------------------------|--------------------------------------------|
| Workbench auth migration     | `workbench-2.x-to-3.x-upgrade.sh patch`              | `workbenches.patch-auth-model`             |
| Workbench OAuth cleanup      | `workbench-2.x-to-3.x-upgrade.sh cleanup`            | `workbenches.cleanup-oauth`                |
| Workbench Kueue label        | `workbench-2.x-to-3.x-upgrade.sh attach-kueue-label` | `workbenches.attach-kueue-label`           |
| Workbench verification       | `workbench-2.x-to-3.x-upgrade.sh verify`             | `workbenches.verify-migration`             |
| Workbench upgrade            | *(manual)*                                           | `workbenches.upgrade-2x-to-3x`             |
| RayCluster backup            | `ray_cluster_migration.py pre-upgrade`               | `raycluster.backup`                        |
| RayCluster migrate           | `ray_cluster_migration.py post-upgrade`              | `raycluster.migrate`                       |
| Serverless to Raw            | `serverless-to-raw.sh`                               | `modelserving.serverless-to-raw`           |
| ModelMesh to Raw             | `modelmesh-to-raw.sh`                                | `modelserving.modelmesh-to-raw`            |
| Hardware profiles ignorelist | `hardwareprofiles-ignorelist.sh`                     | `modelserving.hardwareprofiles-ignorelist` |
| Add owner references         | `add-owner-references.sh`                            | `modelserving.add-owner-references`        |
| Managed ISVC config          | `managed-inferenceservice-config.sh`                 | `modelserving.managed-isvc-config`         |
| AI Pipelines pre-check       | `check_before_upgrade.sh`                            | `ai-pipelines.pre-upgrade-check`           |
| AI Pipelines DSP role        | `update_dsp_role.sh`                                 | `ai-pipelines.update-dsp-role`             |
| AI Pipelines post-check      | `post_upgrade_check.sh`                              | `ai-pipelines.post-upgrade-check`          |
| TrustyAI GPU deadlock        | `break-gpu-deadlock.sh`                              | `trustyai.break-gpu-deadlock`              |
| TrustyAI guardrails          | `patch-guardrails-deployment.sh`                     | `trustyai.patch-guardrails`                |
| TrustyAI OTel exporter       | `migrate-gorch-otel-exporter.sh`                     | `trustyai.migrate-gorch-otel-exporter`     |
| TrustyAI metrics backup      | `backup-metrics.sh` / `restore-metrics.sh`           | `trustyai.metrics`                         |
| TrustyAI data backup         | `backup-data.sh` / `restore-data.sh`                 | `trustyai.data`                            |
| Training verification        | `kubeflow-trainer-verification.sh`                   | `training.verify-workloads`                |
| LlamaStack backup            | `backup-all-llamastack.sh`                           | `llamastack.backup`                        |
| Kueue RHBOK migration        | *(not available)*                                    | `kueue.rhbok.migrate`                      |

---

## Getting Started

### Container (Recommended)

```bash
# Podman
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:ro \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate list --target-version 3.5.0

# Docker
docker run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:ro \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate list --target-version 3.5.0
```

> **Tags:** `:vX.Y.Z` pins a specific release (recommended for migrations, so the pre-upgrade and post-upgrade steps run the same binary). `:latest` tracks the latest stable release and `:dev` tracks the latest development build from `main` -- both can move between your pre-upgrade and post-upgrade steps.

### Token Authentication (no kubeconfig file)

If you don't already have a kubeconfig file, generate one with `oc login` and mount it read-only. This keeps the API token out of the `rhai-cli` command line (and out of shell history and `ps` output):

```bash
oc login --token=sha256~xxxx --server=https://api.my-cluster.example.com:6443 \
  --kubeconfig=/tmp/rhoai-migrate-kubeconfig

podman run --rm -ti \
  -v /tmp/rhoai-migrate-kubeconfig:/kubeconfig:ro \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate list --target-version 3.5.0
```

### SELinux Systems (Fedora, RHEL, CentOS)

Add `:Z` to the volume mount:

```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:ro,Z \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate list --target-version 3.5.0
```

---

## Key Differences

| Aspect              | rhoai-upgrade-helpers                                  | rhai-cli                                            |
|---------------------|--------------------------------------------------------|-----------------------------------------------------|
| **Distribution**    | Git clone + manual deps (`jq`, `yq`, `python3`, `pip`) | Single container image or binary                    |
| **Languages**       | Shell (76%) + Python (24%)                             | Go binary                                           |
| **Discovery**       | Read per-component READMEs                             | `rhai-cli migrate list --target-version X.Y.Z`      |
| **Phase filtering** | Manual (run scripts in order)                          | `--phase pre-upgrade` / `--phase post-upgrade`      |
| **Dry-run**         | Per-script flag (inconsistent)                         | Universal `--dry-run` flag                          |
| **Confirmation**    | Per-script (`-y` or `--yes`)                           | Universal `--yes` flag                              |
| **Scoping**         | Per-script flags (`--name`, `--namespace`, `--all`)    | Component-prefixed flags (e.g. `--workbench-namespace`/`--workbench-name`, `--raycluster-namespace`/`--raycluster-cluster`); actions that always run cluster-wide omit scope flags entirely |
| **Output format**   | Unstructured text                                      | Structured steps with pass/fail status              |
| **Error handling**  | Varies by script                                       | Fail-fast with sequential execution                 |
| **Backup support**  | Manual directory management                            | `migrate prepare` with timestamped output           |
| **Prerequisites**   | `oc`, `jq`, `yq`, `python3` per component              | Only `oc` or `kubectl` (bundled in container)       |

---

## Workbenches

The `workbench-2.x-to-3.x-upgrade.sh` script bundled five subcommands. The CLI breaks these into separate, composable actions that share common scope flags.

**Shared workbench flags** (available on `patch-auth-model`, `cleanup-oauth`, `verify-migration`, and `attach-kueue-label`):

| Flag                         | Description                                                         |
|------------------------------|---------------------------------------------------------------------|
| `--workbench-namespace <ns>` | Limit to notebooks in this namespace (default: all namespaces)      |
| `--workbench-name <name>`    | Target a single notebook by name (requires `--workbench-namespace`) |

### List / Discover Workbenches

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# List all workbenches cluster-wide
./workbench-2.x-to-3.x-upgrade.sh list --all

# List in a specific namespace
./workbench-2.x-to-3.x-upgrade.sh list \
  --namespace my-project
```

</td>
<td>

```bash
# List applicable migrations
rhai-cli migrate list \
  --target-version 3.5.0 \
  --phase pre-upgrade

# Dry-run to preview what would change
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --dry-run
```

</td>
</tr>
</table>

<details>
<summary><b>Example: <code>migrate list</code> output</b></summary>

```
$ rhai-cli migrate list --target-version 3.5.0 --phase pre-upgrade

ID                                NAME                                                    PHASE          APPLICABLE  DESCRIPTION
─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
workbenches.patch-auth-model      Patch workbench auth model from OAuth-proxy to kube-...  pre-upgrade    Yes         Migrates Notebook CRs from oauth-proxy (2.x) to...
workbenches.upgrade-2x-to-3x     Upgrade workbenches from 2.x to 3.x                     pre-upgrade    Yes         Updates workbench notebook container names for 3...
modelserving.serverless-to-raw    Convert Serverless InferenceServices to RawDeployment    pre-upgrade    Yes         Converts InferenceServices using Serverless depl...
ai-pipelines.pre-upgrade-check    AI Pipelines pre-upgrade check                           pre-upgrade    Yes         Captures DSPA pod health, migrates v1alpha1 reso...
raycluster.backup                 Backup RayClusters for RHOAI 3.x migration               pre-upgrade    Yes         Backup RayCluster configurations and run pre-fli...
trustyai.migrate-gorch-otel-...   Migrate GuardrailsOrchestrator otelExporter schema       pre-upgrade    Yes         Migrate otelExporter from RHOAI 2.25 schema to c...
kueue.rhbok.migrate               Migrate Kueue to Red Hat build of Kueue                  pre-upgrade    Yes         Migrates from OpenShift AI built-in Kueue to Red...
llamastack.backup                 Backup LlamaStack Resources                              pre-upgrade    Yes         Backup all LlamaStack resources before upgrade (L...
training.verify-workloads         Verify training workloads                                pre-upgrade    Yes         Pre-upgrade check for Kubeflow v1 training workl...

Note: 'migrate run' will auto-detect phase as "pre-upgrade" for this cluster.
Use --phase to filter the list by lifecycle phase.
```

</details>

### Patch Auth Model (OAuth-proxy to kube-rbac-proxy)

The `patch-auth-model` action migrates Notebook CRs by applying seven patches: adding `inject-auth` annotation, removing `inject-oauth` and `oauth-logout-url` annotations, removing the `oauth-proxy` sidecar container, removing OAuth finalizers and volumes, and stripping `tornado_settings` from `NOTEBOOK_ARGS`.

**Action-specific flags:**

| Flag             | Description                                                                             |
|------------------|-----------------------------------------------------------------------------------------|
| `--skip-stop`    | Skip auto stop/restart of running workbenches before patching                           |
| `--only-stopped` | Only patch stopped workbenches, skip running ones                                       |
| `--with-cleanup` | Chain OAuth cleanup after patching (delete legacy Route, Service, Secrets, OAuthClient) |

> **Note:** `--skip-stop` and `--only-stopped` are mutually exclusive.

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Patch a single workbench
./workbench-2.x-to-3.x-upgrade.sh patch \
  --name my-wb --namespace my-project

# Patch all workbenches cluster-wide
./workbench-2.x-to-3.x-upgrade.sh patch --all

# Skip stop/restart
./workbench-2.x-to-3.x-upgrade.sh patch \
  --all --skip-stop

# Patch + cleanup in one step
./workbench-2.x-to-3.x-upgrade.sh patch \
  --all --with-cleanup

# Non-interactive (CI)
./workbench-2.x-to-3.x-upgrade.sh patch \
  --all -y
```

</td>
<td>

```bash
# Patch a single workbench
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --workbench-name my-wb \
  --workbench-namespace my-project

# Patch all workbenches cluster-wide
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0

# Skip stop/restart
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --skip-stop

# Only patch stopped workbenches
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --only-stopped

# Patch + cleanup in one step
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --with-cleanup

# Dry-run first
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --dry-run

# Non-interactive (CI)
rhai-cli migrate run \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --yes

# Backup notebooks before patching
rhai-cli migrate prepare \
  --migration workbenches.patch-auth-model \
  --target-version 3.5.0 \
  --output-dir /backups
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Patch auth model output (with auto-stop lifecycle)</b></summary>

```
$ rhai-cli migrate run --migration workbenches.patch-auth-model --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

workbenches.patch-auth-model:

Running migration: workbenches.patch-auth-model (confirmations skipped)

  → Discover notebooks for migration
    ✓ Found 5 Notebooks across 3 namespaces (4 need patching, 1 already migrated)
  → Stop running workbench my-wb (my-project)
    ✓ Annotated kubeflow-resource-stopped
    ✓ StatefulSet scaled down
  → Patch Notebook my-wb (my-project)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet my-wb
  → Patch Notebook data-analysis (team-ds)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet data-analysis
  → Patch Notebook training-nb (team-ml)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet training-nb
  → Notebook feature-eng (team-ds)
    → Already migrated (skipped)
  → Patch Notebook llm-eval (team-ml)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet llm-eval
  → Restart workbench my-wb (my-project)
    ✓ Removed kubeflow-resource-stopped annotation

Migration workbenches.patch-auth-model completed successfully!
```

</details>

<details>
<summary><b>Example: Patch with <code>--with-cleanup</code> (chained cleanup)</b></summary>

```
$ rhai-cli migrate run --migration workbenches.patch-auth-model --target-version 3.5.0 \
    --workbench-namespace my-project --with-cleanup --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

workbenches.patch-auth-model:

Running migration: workbenches.patch-auth-model (confirmations skipped)

  → Discover notebooks for migration
    ✓ Found 2 Notebooks in namespace my-project (2 need patching)
  → Stop running workbench my-wb (my-project)
    ✓ StatefulSet scaled down
  → Patch Notebook my-wb (my-project)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet my-wb
  → Patch Notebook data-analysis (my-project)
    ✓ Added annotation inject-auth=true
    ✓ Removed oauth-proxy sidecar container
    ✓ Removed OAuth annotations, volumes, and finalizers
    ✓ Stripped tornado_settings from NOTEBOOK_ARGS
    ✓ Deleted StatefulSet data-analysis
  → Cleanup OAuth resources for my-wb (my-project)
    ✓ Deleted Route my-wb
    ✓ Deleted Service my-wb-tls
    ✓ Deleted Secret my-wb-oauth-client
    ✓ Deleted Secret my-wb-oauth-config
    ✓ Deleted Secret my-wb-tls
    ✓ Deleted OAuthClient my-wb-my-project-oauth-client
  → Cleanup OAuth resources for data-analysis (my-project)
    ✓ Deleted Route data-analysis
    ✓ Deleted Service data-analysis-tls
    ✓ Deleted Secret data-analysis-oauth-client
    ✓ Deleted Secret data-analysis-oauth-config
    ✓ Deleted Secret data-analysis-tls
    ✓ Deleted OAuthClient data-analysis-my-project-oauth-client
  → Restart workbench my-wb (my-project)
    ✓ Removed kubeflow-resource-stopped annotation

Migration workbenches.patch-auth-model completed successfully!
```

</details>

<details>
<summary><b>Example: Dry-run output</b></summary>

```
$ rhai-cli migrate run --migration workbenches.patch-auth-model --target-version 3.5.0 --dry-run

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

DRY RUN MODE: No changes will be made to the cluster

workbenches.patch-auth-model:

  → Discover notebooks for migration
    ✓ Found 5 Notebooks across 3 namespaces (4 need patching, 1 already migrated)
  → Would patch Notebook my-wb (my-project)
    → Would add annotation inject-auth=true
    → Would remove oauth-proxy sidecar container
    → Would remove OAuth annotations, volumes, and finalizers
    → Would delete StatefulSet my-wb
  → Would patch Notebook data-analysis (team-ds)
    → Would add annotation inject-auth=true
    → Would remove oauth-proxy sidecar container
    → Would remove OAuth annotations, volumes, and finalizers
    → Would delete StatefulSet data-analysis
  → Notebook feature-eng (team-ds)
    → Already migrated (skipped)

Migration workbenches.patch-auth-model completed successfully!
```

</details>

<details>
<summary><b>Example: Backup notebooks before patching (<code>migrate prepare</code>)</b></summary>

```
$ rhai-cli migrate prepare --migration workbenches.patch-auth-model --target-version 3.5.0 --output-dir /backups

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade
Backup directory: /backups/backup-migrate-20250708-153045

workbenches.patch-auth-model:

  → Discover notebooks for migration
    ✓ Found 5 Notebooks across 3 namespaces (4 need patching, 1 already migrated)
  → Backup Notebook my-wb (my-project)
    ✓ Saved my-project/my-wb.yaml
  → Backup Notebook data-analysis (team-ds)
    ✓ Saved team-ds/data-analysis.yaml
  → Backup Notebook training-nb (team-ml)
    ✓ Saved team-ml/training-nb.yaml
  → Backup Notebook llm-eval (team-ml)
    ✓ Saved team-ml/llm-eval.yaml

Preparation workbenches.patch-auth-model completed successfully!

All preparations completed successfully!
Backups saved to: /backups/backup-migrate-20250708-153045

Run 'migrate run' to execute the migration.
```

</details>

### Upgrade Workbenches (2.x to 3.x Container Fix)

This action has no bash script equivalent -- it was previously a manual step. It fixes a container name mismatch in Dashboard-managed Notebooks: when the primary workload container's name doesn't match the Notebook CR name, accelerator injection and size selection stop working after upgrading to RHOAI 3.x. It applies to clusters upgrading from 2.16+ to 3.x.

> **Note:** Unlike the other workbench actions, `upgrade-2x-to-3x` does **not** support `--workbench-namespace` or `--workbench-name` scoping -- it always discovers and fixes Notebooks across all namespaces.

**Action-specific flag:**

| Flag                  | Description                                                                                                                                                                     |
|-----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `--force-non-stopped` | Include non-stopped (running) workbenches in migration. **Unsafe: may cause data loss.** By default, non-stopped Notebooks are skipped and reported so you can stop them first. |

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# No equivalent -- previously required manually
# stopping workbenches and inspecting/patching
# container names by hand.
```

</td>
<td>

```bash
# Dry-run to preview which Notebooks need fixing
rhai-cli migrate run \
  --migration workbenches.upgrade-2x-to-3x \
  --target-version 3.5.0 \
  --dry-run

# Fix stopped Notebooks (default: skips non-stopped ones)
rhai-cli migrate run \
  --migration workbenches.upgrade-2x-to-3x \
  --target-version 3.5.0

# Include running Notebooks too (unsafe: may cause data loss)
rhai-cli migrate run \
  --migration workbenches.upgrade-2x-to-3x \
  --target-version 3.5.0 \
  --force-non-stopped

# Non-interactive (CI)
rhai-cli migrate run \
  --migration workbenches.upgrade-2x-to-3x \
  --target-version 3.5.0 \
  --yes
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Upgrade workbenches output</b></summary>

```
$ rhai-cli migrate run --migration workbenches.upgrade-2x-to-3x --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

workbenches.upgrade-2x-to-3x:

Running migration: workbenches.upgrade-2x-to-3x (confirmations skipped)

  → Discover notebooks for migration
    ✓ Found 4 Dashboard-managed Notebook(s) with container name mismatches
  → Check for non-stopped workbenches
    ✓ All 4 Notebook(s) are stopped
  → Fix container name for my-wb (my-project)
    ✓ Renamed container my-wb-notebook to my-wb
  → Fix container name for data-analysis (team-ds)
    ✓ Renamed container data-analysis-notebook to data-analysis

Migration workbenches.upgrade-2x-to-3x completed successfully!
```

</details>

### Cleanup Legacy OAuth Resources

Removes stale OAuth-proxy resources left behind after patching: Route, Service (`-tls`), Secrets (`-oauth-client`, `-oauth-config`, `-tls`), and OAuthClient.

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Cleanup a single workbench
./workbench-2.x-to-3.x-upgrade.sh cleanup \
  --name my-wb --namespace my-project

# Cleanup all workbenches
./workbench-2.x-to-3.x-upgrade.sh cleanup --all

# Non-interactive
./workbench-2.x-to-3.x-upgrade.sh cleanup \
  --all -y
```

</td>
<td>

```bash
# Cleanup a single workbench
rhai-cli migrate run \
  --migration workbenches.cleanup-oauth \
  --target-version 3.5.0 \
  --workbench-name my-wb \
  --workbench-namespace my-project

# Cleanup all workbenches cluster-wide
rhai-cli migrate run \
  --migration workbenches.cleanup-oauth \
  --target-version 3.5.0

# Limit to a namespace
rhai-cli migrate run \
  --migration workbenches.cleanup-oauth \
  --target-version 3.5.0 \
  --workbench-namespace my-project

# Non-interactive
rhai-cli migrate run \
  --migration workbenches.cleanup-oauth \
  --target-version 3.5.0 \
  --yes
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Cleanup OAuth output</b></summary>

```
$ rhai-cli migrate run --migration workbenches.cleanup-oauth --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

workbenches.cleanup-oauth:

Running migration: workbenches.cleanup-oauth (confirmations skipped)

  → Discover notebooks for cleanup
    ✓ Found 3 Notebooks with stale OAuth resources
  → Cleanup Notebook my-wb (my-project)
    ✓ Deleted Route my-wb
    ✓ Deleted Service my-wb-tls
    ✓ Deleted Secret my-wb-oauth-client
    ✓ Deleted Secret my-wb-oauth-config
    ✓ Deleted Secret my-wb-tls
    ✓ Deleted OAuthClient my-wb-my-project-oauth-client
  → Cleanup Notebook data-analysis (team-ds)
    ✓ Deleted Route data-analysis
    ✓ Deleted Service data-analysis-tls
    ✓ Deleted Secret data-analysis-oauth-client
    ✓ Deleted Secret data-analysis-oauth-config
    ✓ Deleted Secret data-analysis-tls
    ✓ Deleted OAuthClient data-analysis-team-ds-oauth-client
  → Cleanup Notebook training-nb (team-ml)
    ✓ Deleted Route training-nb
    ✓ Deleted Service training-nb-tls
    ✓ Deleted Secret training-nb-oauth-client
    ✓ Deleted Secret training-nb-oauth-config
    → Secret training-nb-tls not found (already removed)
    ✓ Deleted OAuthClient training-nb-team-ml-oauth-client

Migration workbenches.cleanup-oauth completed successfully!
```

</details>

### Verify Migration Status

The verify action is read-only -- it classifies each notebook by migration state and reports results without modifying anything.

**Classification states:**

| Status         | Meaning                                                              |
|----------------|----------------------------------------------------------------------|
| `MIGRATED`     | Has kube-rbac-proxy sidecar, no oauth-proxy -- successfully migrated |
| `LEGACY`       | Still using oauth-proxy auth model -- needs migration                |
| `UNRECONCILED` | Has `inject-auth` annotation but oauth-proxy sidecar still present   |
| `INVALID`      | Both kube-rbac-proxy and oauth-proxy sidecars present                |
| `UNKNOWN`      | No auth sidecars found or unable to determine                        |

**Action-specific flag:**

| Flag                     | Description                                               |
|--------------------------|-----------------------------------------------------------|
| `--verify-phase <phase>` | What to check: `migration` (default), `cleanup`, or `all` |

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Verify migration status
./workbench-2.x-to-3.x-upgrade.sh verify \
  --phase migration --all

# Verify cleanup status
./workbench-2.x-to-3.x-upgrade.sh verify \
  --phase cleanup --all

# Verify both
./workbench-2.x-to-3.x-upgrade.sh verify \
  --phase all --all
```

</td>
<td>

```bash
# Verify migration status (default)
rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0

# Verify cleanup status
rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0 \
  --verify-phase cleanup

# Verify both migration and cleanup
rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0 \
  --verify-phase all

# Scope to a namespace
rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0 \
  --workbench-namespace my-project

# Single notebook
rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0 \
  --workbench-name my-wb \
  --workbench-namespace my-project
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Verify migration output</b></summary>

```
$ rhai-cli migrate run --migration workbenches.verify-migration --target-version 3.5.0 --verify-phase all

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

workbenches.verify-migration:

Running migration: workbenches.verify-migration

  → Discover notebooks for migration verification
    ✓ Found 5 Notebook(s) to verify
  → Verify my-project/my-wb
    ✓ Status: MIGRATED | State: Running
    ✓ Migration checks: all passed
    ✓ Cleanup checks: all passed
    ✓ All checks passed for my-project/my-wb [MIGRATED]
  → Verify team-ds/data-analysis
    ✓ Status: MIGRATED | State: Stopped
    ✓ Migration checks: all passed
    ✓ Cleanup checks: all passed
    ✓ All checks passed for team-ds/data-analysis [MIGRATED]
  → Verify team-ml/training-nb
    ✓ Status: MIGRATED | State: Running
    ✓ Migration checks: all passed
    ✗ Cleanup checks failed: Route training-nb still exists; Service training-nb-tls still exists
    ✗ Some checks failed for team-ml/training-nb [MIGRATED]
  → Verify team-ds/feature-eng
    ✓ Status: MIGRATED | State: Running
    ✓ Migration checks: all passed
    ✓ Cleanup checks: all passed
    ✓ All checks passed for team-ds/feature-eng [MIGRATED]
  → Verify team-ml/llm-eval
    ✓ Status: LEGACY | State: Running
    ✗ Migration checks failed: inject-auth annotation missing; oauth-proxy sidecar still present
    ✗ Cleanup checks failed: Route llm-eval still exists
    ✗ Some checks failed for team-ml/llm-eval [LEGACY]
  → Verification summary
    ✗ Verified 5 Notebook(s): 2 with failures

Migration workbenches.verify-migration completed with skipped steps
```

</details>

### Attach Kueue Label

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./workbench-2.x-to-3.x-upgrade.sh \
  attach-kueue-label --all
```

</td>
<td>

```bash
# All workbenches cluster-wide
rhai-cli migrate run \
  --migration workbenches.attach-kueue-label \
  --target-version 3.5.0

# Scope to a namespace
rhai-cli migrate run \
  --migration workbenches.attach-kueue-label \
  --target-version 3.5.0 \
  --workbench-namespace my-project
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Attach Kueue label output</b></summary>

```
$ rhai-cli migrate run --migration workbenches.attach-kueue-label --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

workbenches.attach-kueue-label:

Running migration: workbenches.attach-kueue-label (confirmations skipped)

  → Detect Kueue-managed namespaces
    ✓ Found 2 namespaces with LocalQueue resources
  → List Notebooks in Kueue-managed namespaces
    ✓ Found 3 Notebooks
  → Label Notebook data-analysis (team-ds)
    ✓ Added label kueue.x-k8s.io/queue-name=default-queue
  → Label Notebook feature-eng (team-ds)
    → Already labeled (skipped)
  → Label Notebook training-nb (team-ml)
    ✓ Added label kueue.x-k8s.io/queue-name=default-queue

Migration workbenches.attach-kueue-label completed successfully!
```

</details>

---

## RayCluster

The Python script `ray_cluster_migration.py` required Python 3, pip, and the `kubernetes` + `PyYAML` packages. The CLI handles this natively.

### Pre-Upgrade Backup

<table>
<tr><th>Python Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Install dependencies
pip install -r ray_cluster_migration_requirements.txt

# Backup all RayClusters
python ray_cluster_migration.py pre-upgrade

# Backup a specific namespace
python ray_cluster_migration.py pre-upgrade \
  --namespace my-ns

# Backup a single cluster
python ray_cluster_migration.py pre-upgrade \
  --cluster my-rc --namespace my-ns

# Custom backup directory
RHOAI_UPGRADE_BACKUP_DIR=/backups \
  python ray_cluster_migration.py pre-upgrade
```

</td>
<td>

```bash
# Backup all RayClusters
rhai-cli migrate run \
  --migration raycluster.backup \
  --target-version 3.5.0

# Backup a specific namespace
rhai-cli migrate run \
  --migration raycluster.backup \
  --target-version 3.5.0 \
  --raycluster-namespace my-ns

# Backup a single cluster
rhai-cli migrate run \
  --migration raycluster.backup \
  --target-version 3.5.0 \
  --raycluster-cluster my-rc --raycluster-namespace my-ns

# Use the prepare command for backups
rhai-cli migrate prepare \
  --migration raycluster.backup \
  --target-version 3.5.0 \
  --output-dir /backups

# Dry-run
rhai-cli migrate run \
  --migration raycluster.backup \
  --target-version 3.5.0 \
  --dry-run
```

</td>
</tr>
</table>

<details>
<summary><b>Example: RayCluster backup (<code>migrate prepare</code>) output</b></summary>

```
$ rhai-cli migrate prepare --migration raycluster.backup --target-version 3.5.0 --output-dir /backups

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade
Backup directory: /backups/backup-migrate-20250708-153045

raycluster.backup:

  → Pre-flight checks
    ✓ cert-manager is installed (required for RHOAI 3.x)
  → List RayClusters
    ✓ Found 3 RayClusters across 2 namespaces
  → Backup RayCluster my-rc (my-ns)
    ✓ Saved rhoai-2.x/raycluster-my-rc-my-ns.yaml
    ✓ Saved rhoai-3.x/raycluster-my-rc-my-ns.yaml (CodeFlare components removed)
  → Backup RayCluster training-ray (team-ml)
    ✓ Saved rhoai-2.x/raycluster-training-ray-team-ml.yaml
    ✓ Saved rhoai-3.x/raycluster-training-ray-team-ml.yaml (CodeFlare components removed)
  → Backup RayCluster inference-ray (team-ds)
    ✓ Saved rhoai-2.x/raycluster-inference-ray-team-ds.yaml
    ✓ Saved rhoai-3.x/raycluster-inference-ray-team-ds.yaml (CodeFlare components removed)

Preparation raycluster.backup completed successfully!

All preparations completed successfully!
Backups saved to: /backups/backup-migrate-20250708-153045

Run 'migrate run' to execute the migration.
```

</details>

### Post-Upgrade Migration

<table>
<tr><th>Python Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Dry-run first
python ray_cluster_migration.py post-upgrade \
  --cluster my-rc --namespace my-ns --dry-run

# Migrate a single cluster (live in-place)
python ray_cluster_migration.py post-upgrade \
  --cluster my-rc --namespace my-ns

# Migrate from backup
python ray_cluster_migration.py post-upgrade \
  --cluster my-rc --namespace my-ns \
  --from-backup

# Migrate all clusters
python ray_cluster_migration.py post-upgrade

# List migration status
python ray_cluster_migration.py list

# Non-interactive
python ray_cluster_migration.py post-upgrade \
  --yes
```

</td>
<td>

```bash
# Dry-run first
rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0 \
  --raycluster-cluster my-rc --raycluster-namespace my-ns \
  --dry-run

# Migrate a single cluster (live in-place)
rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0 \
  --raycluster-cluster my-rc --raycluster-namespace my-ns

# Migrate from backup (deletes existing cluster first)
rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0 \
  --raycluster-cluster my-rc --raycluster-namespace my-ns \
  --raycluster-from-backup /backups/backup-migrate-20250708-153045

# Migrate all clusters
rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0

# Non-interactive
rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0 \
  --yes
```

</td>
</tr>
</table>

<details>
<summary><b>Example: RayCluster post-upgrade migration output</b></summary>

```
$ rhai-cli migrate run --migration raycluster.migrate --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

raycluster.migrate:

Running migration: raycluster.migrate (confirmations skipped)

  → List RayClusters
    ✓ Found 3 RayClusters across 2 namespaces (1 already migrated, 2 need migration)
  → Migrate RayCluster my-rc (my-ns)
    ✓ Removed CodeFlare TLS/OAuth components from spec
    ✓ KubeRay recreated head pod
    ✓ KubeRay recreated 2 worker pods
    ✓ Gateway API route: https://ray-dashboard-my-rc-my-ns.apps.cluster.example.com
  → Migrate RayCluster training-ray (team-ml)
    ✓ Removed CodeFlare TLS/OAuth components from spec
    ✓ KubeRay recreated head pod
    ✓ KubeRay recreated 4 worker pods
    ✓ Gateway API route: https://ray-dashboard-training-ray-team-ml.apps.cluster.example.com
  → RayCluster inference-ray (team-ds)
    → Already migrated (skipped)

Migration raycluster.migrate completed successfully!
```

</details>

---

## Model Serving

The bash scripts were spread across `model-serving/before-upgrade/` and `model-serving/after-upgrade/`.

### Serverless to RawDeployment

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Prerequisites: oc, yq, jq

# Dry-run
./serverless-to-raw.sh --dry-run

# Migrate in a specific namespace
./serverless-to-raw.sh -n my-ns

# Migrate all namespaces
./serverless-to-raw.sh
```

</td>
<td>

```bash
# Dry-run
rhai-cli migrate run \
  --migration modelserving.serverless-to-raw \
  --target-version 3.5.0 \
  --dry-run

# Migrate (operates on all namespaces)
rhai-cli migrate run \
  --migration modelserving.serverless-to-raw \
  --target-version 3.5.0
```

> **Note:** The CLI always operates cluster-wide.
> The bash script's `-n`/`--namespace` flag has no rhai-cli equivalent.

</td>
</tr>
</table>

<details>
<summary><b>Example: Serverless to Raw output</b></summary>

```
$ rhai-cli migrate run --migration modelserving.serverless-to-raw --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

modelserving.serverless-to-raw:

Running migration: modelserving.serverless-to-raw (confirmations skipped)

  → Discover Serverless InferenceServices
    ✓ Found 4 InferenceServices using Serverless deployment mode
  → Convert ISVC text-gen-llm (model-serving)
    ✓ Exported original config to original/text-gen-llm.yaml
    ✓ Created RawDeployment ISVC text-gen-llm-raw
    ✓ Created ServiceAccount text-gen-llm-raw-sa
    ✓ Created Role text-gen-llm-raw-role
    ✓ Created RoleBinding text-gen-llm-raw-rolebinding
  → Convert ISVC embeddings-model (model-serving)
    ✓ Exported original config to original/embeddings-model.yaml
    ✓ Created RawDeployment ISVC embeddings-model-raw
    ✓ Created ServiceAccount embeddings-model-raw-sa
    ✓ Created Role embeddings-model-raw-role
    ✓ Created RoleBinding embeddings-model-raw-rolebinding
  → Convert ISVC classification (ml-prod)
    ✓ Exported original config to original/classification.yaml
    ✓ Created RawDeployment ISVC classification-raw
    ✓ Created ServiceAccount classification-raw-sa
    ✓ Created Role classification-raw-role
    ✓ Created RoleBinding classification-raw-rolebinding
  → Convert ISVC fraud-detect (ml-prod)
    ✓ Exported original config to original/fraud-detect.yaml
    ✓ Created RawDeployment ISVC fraud-detect-raw
    ✓ Created ServiceAccount fraud-detect-raw-sa
    ✓ Created Role fraud-detect-raw-role
    ✓ Created RoleBinding fraud-detect-raw-rolebinding

Migration modelserving.serverless-to-raw completed successfully!
```

</details>

### ModelMesh to RawDeployment

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Dry-run
./modelmesh-to-raw.sh --dry-run

# In-place migration (destructive)
./modelmesh-to-raw.sh \
  --from-ns my-ns --preserve-namespace

# Migrate to new namespace
./modelmesh-to-raw.sh \
  --from-ns old-ns --target-ns new-ns

# ODH mode
./modelmesh-to-raw.sh --odh
```

</td>
<td>

```bash
# Dry-run
rhai-cli migrate run \
  --migration modelserving.modelmesh-to-raw \
  --target-version 3.5.0 \
  --dry-run

# Migrate (operates on all namespaces)
rhai-cli migrate run \
  --migration modelserving.modelmesh-to-raw \
  --target-version 3.5.0
```

> **Note:** The CLI always operates cluster-wide and in-place.
> The bash script's `--target-ns`, `--preserve-namespace`, and `--odh`
> flags have no rhai-cli equivalents.

</td>
</tr>
</table>

<details>
<summary><b>Example: ModelMesh to Raw output</b></summary>

```
$ rhai-cli migrate run --migration modelserving.modelmesh-to-raw --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

modelserving.modelmesh-to-raw:

Running migration: modelserving.modelmesh-to-raw (confirmations skipped)

  → Discover ModelMesh InferenceServices across all namespaces
    ✓ Found 2 InferenceServices using ModelMesh
  → Convert ISVC sklearn-model (my-ns)
    ✓ Transformed ServingRuntime to KServe format
    ✓ Copied storage secrets
    ✓ Created RawDeployment ISVC sklearn-model
    ✓ Created auth resources (SA, Role, RoleBinding)
    ✓ Exposed route for sklearn-model
  → Convert ISVC xgboost-predictor (my-ns)
    ✓ Transformed ServingRuntime to KServe format
    ✓ Copied storage secrets
    ✓ Created RawDeployment ISVC xgboost-predictor
    ✓ Created auth resources (SA, Role, RoleBinding)
    ✓ Exposed route for xgboost-predictor

Migration modelserving.modelmesh-to-raw completed successfully!
```

</details>

### Hardware Profiles Ignorelist

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./hardwareprofiles-ignorelist.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration modelserving.hardwareprofiles-ignorelist \
  --target-version 3.5.0
```

</td>
</tr>
</table>

### Add Owner References

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./add-owner-references.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration modelserving.add-owner-references \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Hardware profiles ignorelist and add owner references output</b></summary>

```
$ rhai-cli migrate run --migration modelserving.hardwareprofiles-ignorelist --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

modelserving.hardwareprofiles-ignorelist:

Running migration: modelserving.hardwareprofiles-ignorelist (confirmations skipped)

  → Patch inferenceservice-config ConfigMap
    ✓ Set annotation opendatahub.io/managed=false
    ✓ Added hardware-profile annotations to serviceAnnotationDisallowedList

Migration modelserving.hardwareprofiles-ignorelist completed successfully!
```

```
$ rhai-cli migrate run --migration modelserving.add-owner-references --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

modelserving.add-owner-references:

Running migration: modelserving.add-owner-references (confirmations skipped)

  → Discover RawDeployment InferenceServices
    ✓ Found 6 RawDeployment ISVCs across 3 namespaces
  → Patch auth resources for text-gen-llm-raw (model-serving)
    ✓ Added ownerReference to ServiceAccount text-gen-llm-raw-sa
    ✓ Added ownerReference to Role text-gen-llm-raw-role
    ✓ Added ownerReference to RoleBinding text-gen-llm-raw-rolebinding
  → Patch auth resources for embeddings-model-raw (model-serving)
    ✓ Added ownerReference to ServiceAccount embeddings-model-raw-sa
    ✓ Added ownerReference to Role embeddings-model-raw-role
    ✓ Added ownerReference to RoleBinding embeddings-model-raw-rolebinding
  → Patch auth resources for classification-raw (ml-prod)
    → Already has ownerReferences (skipped)

Migration modelserving.add-owner-references completed successfully!
```

</details>

### Managed ISVC Config (Post-Upgrade)

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./managed-inferenceservice-config.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration modelserving.managed-isvc-config \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Managed ISVC config (post-upgrade) output</b></summary>

```
$ rhai-cli migrate run --migration modelserving.managed-isvc-config --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

modelserving.managed-isvc-config:

Running migration: modelserving.managed-isvc-config (confirmations skipped)

  → Patch inferenceservice-config ConfigMap
    ✓ Set annotation opendatahub.io/managed=true
  → Restart KServe controller
    ✓ Restarted kserve-controller-manager deployment
    ✓ Controller pod ready

Migration modelserving.managed-isvc-config completed successfully!
```

</details>

---

## AI Pipelines

### Pre-Upgrade Check

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Prerequisites: kubectl, jq

# Run pre-upgrade checks
./check_before_upgrade.sh

# Custom state file location
DSPA_STATE_FILE=/tmp/my-state.json \
  ./check_before_upgrade.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration ai-pipelines.pre-upgrade-check \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: AI Pipelines pre-upgrade check output</b></summary>

```
$ rhai-cli migrate run --migration ai-pipelines.pre-upgrade-check --target-version 3.5.0

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

ai-pipelines.pre-upgrade-check:

Running migration: ai-pipelines.pre-upgrade-check

  → Capture DSPA pod health baseline
    ✓ Saved pod health state for 4 DSPAs across 3 namespaces
  → Migrate v1alpha1 DSPAs to v1
    ✓ Migrated 2 DSPAs from v1alpha1 to v1
    → 2 DSPAs already using v1 (skipped)
  → Detect custom RBAC roles
    ✓ Found 1 custom role requiring update
    → Run ai-pipelines.update-dsp-role to patch: ds-pipeline-custom (team-ml)

Migration ai-pipelines.pre-upgrade-check completed successfully!
```

</details>

### Update DSP Role

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./update_dsp_role.sh ROLE_NAME NAMESPACE \
  DSPA_NAME VERBS
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration ai-pipelines.update-dsp-role \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Update DSP role output</b></summary>

```
$ rhai-cli migrate run --migration ai-pipelines.update-dsp-role --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

ai-pipelines.update-dsp-role:

Running migration: ai-pipelines.update-dsp-role (confirmations skipped)

  → Discover custom DSP roles
    ✓ Found 1 custom role to patch
  → Patch Role ds-pipeline-custom (team-ml)
    ✓ Added datasciencepipelinesapplications/api subresource
    ✓ Validated patched role matches expected rules

Migration ai-pipelines.update-dsp-role completed successfully!
```

</details>

### Post-Upgrade Check

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Must use same state file as pre-upgrade
DSPA_STATE_FILE=/tmp/dspa_pre_upgrade_pods.json \
  DSPA_WAIT_TIMEOUT=120 \
  ./post_upgrade_check.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration ai-pipelines.post-upgrade-check \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: AI Pipelines post-upgrade check output</b></summary>

```
$ rhai-cli migrate run --migration ai-pipelines.post-upgrade-check --target-version 3.5.0

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

ai-pipelines.post-upgrade-check:

Running migration: ai-pipelines.post-upgrade-check

  → Load pre-upgrade pod health baseline
    ✓ Loaded baseline for 4 DSPAs
  → Compare pod health for dspa-default (pipelines-ns)
    ✓ All 3 pods healthy (matches baseline)
  → Compare pod health for dspa-team-ml (team-ml)
    ✓ All 2 pods healthy (matches baseline)
  → Compare pod health for dspa-analytics (analytics)
    ⋯ Waiting for pods to recover (1/2 healthy)
    ✓ All 2 pods healthy (matches baseline)
  → Compare pod health for dspa-staging (staging)
    ✓ All 1 pods healthy (matches baseline)

Migration ai-pipelines.post-upgrade-check completed successfully!
```

</details>

---

## TrustyAI

Seven separate bash scripts become five CLI actions with consistent interfaces.

### Break GPU Deadlock

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Check for deadlocks
./break-gpu-deadlock.sh --check

# Fix deadlocks
./break-gpu-deadlock.sh --fix
```

</td>
<td>

```bash
# Dry-run (check only)
rhai-cli migrate run \
  --migration trustyai.break-gpu-deadlock \
  --target-version 3.5.0 \
  --dry-run

# Fix deadlocks
rhai-cli migrate run \
  --migration trustyai.break-gpu-deadlock \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Break GPU deadlock output</b></summary>

```
$ rhai-cli migrate run --migration trustyai.break-gpu-deadlock --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

trustyai.break-gpu-deadlock:

Running migration: trustyai.break-gpu-deadlock (confirmations skipped)

  → Detect GPU deadlocks
    ✓ Found 1 deadlocked InferenceService
  → Fix deadlock for llm-predictor (model-serving)
    ✓ Deleted pending pod llm-predictor-predictor-xyz12
    ✓ Re-created pod llm-predictor-predictor-abc34
    ✓ Pod running and serving traffic

Migration trustyai.break-gpu-deadlock completed successfully!
```

</details>

### Patch Guardrails Deployment

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Check
./patch-guardrails-deployment.sh --check

# Dry-run
./patch-guardrails-deployment.sh --dry-run

# Apply fix
./patch-guardrails-deployment.sh --fix
```

</td>
<td>

```bash
# Dry-run
rhai-cli migrate run \
  --migration trustyai.patch-guardrails \
  --target-version 3.5.0 \
  --dry-run

# Apply fix
rhai-cli migrate run \
  --migration trustyai.patch-guardrails \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Patch guardrails output</b></summary>

```
$ rhai-cli migrate run --migration trustyai.patch-guardrails --target-version 3.5.0 --yes

Current OpenShift AI version: 3.5.0
Target OpenShift AI version: 3.5.0
Phase: post-upgrade

trustyai.patch-guardrails:

Running migration: trustyai.patch-guardrails (confirmations skipped)

  → Discover GuardrailsOrchestrator deployments
    ✓ Found 2 deployments missing readiness probe
  → Patch Deployment guardrails-orchestrator (trustyai-ns)
    ✓ Added readinessProbe on port 8034 at /health
  → Patch Deployment guardrails-orch-prod (ml-prod)
    ✓ Added readinessProbe on port 8034 at /health

Migration trustyai.patch-guardrails completed successfully!
```

</details>

### Migrate OTel Exporter

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Dry-run
./migrate-gorch-otel-exporter.sh --dry-run

# Apply
./migrate-gorch-otel-exporter.sh --fix
```

</td>
<td>

```bash
# Dry-run
rhai-cli migrate run \
  --migration trustyai.migrate-gorch-otel-exporter \
  --target-version 3.5.0 \
  --dry-run

# Apply
rhai-cli migrate run \
  --migration trustyai.migrate-gorch-otel-exporter \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Migrate OTel exporter output</b></summary>

```
$ rhai-cli migrate run --migration trustyai.migrate-gorch-otel-exporter --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

trustyai.migrate-gorch-otel-exporter:

Running migration: trustyai.migrate-gorch-otel-exporter (confirmations skipped)

  → Discover GuardrailsOrchestrators with legacy otelExporter spec
    ✓ Found 1 GuardrailsOrchestrator with 2.25 otelExporter schema
  → Migrate GuardrailsOrchestrator guardrails-orchestrator (trustyai-ns)
    ✓ Migrated spec.otelExporter from 2.25 schema to current schema

Migration trustyai.migrate-gorch-otel-exporter completed successfully!
```

</details>

### Backup and Restore Metrics

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Backup (pre-upgrade)
./backup-metrics.sh

# Restore (post-upgrade)
./restore-metrics.sh --dry-run
./restore-metrics.sh --skip-existing
```

</td>
<td>

```bash
# Backup and restore handled by single action
rhai-cli migrate run \
  --migration trustyai.metrics \
  --target-version 3.5.0

# Prepare (backup only)
rhai-cli migrate prepare \
  --migration trustyai.metrics \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: TrustyAI metrics backup/restore output</b></summary>

```
$ rhai-cli migrate prepare --migration trustyai.metrics --target-version 3.5.0

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade
Backup directory: ./backup-migrate-20250708-153045

trustyai.metrics:

  → Discover TrustyAIService instances
    ✓ Found 2 TrustyAIService instances
  → Backup scheduled metrics for trustyai-service (trustyai-ns)
    ✓ Saved 8 scheduled metrics to trustyai-ns-metrics-20250708-153045.json
  → Backup scheduled metrics for trustyai-service (ml-prod)
    ✓ Saved 3 scheduled metrics to ml-prod-metrics-20250708-153045.json

Preparation trustyai.metrics completed successfully!

All preparations completed successfully!
Backups saved to: ./backup-migrate-20250708-153045

Run 'migrate run' to execute the migration.
```

</details>

### Backup and Restore Data

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Backup (pre-upgrade)
./backup-data.sh

# Restore (post-upgrade)
./restore-data.sh
```

</td>
<td>

```bash
# Backup and restore handled by single action
rhai-cli migrate run \
  --migration trustyai.data \
  --target-version 3.5.0

# Prepare (backup only)
rhai-cli migrate prepare \
  --migration trustyai.data \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: TrustyAI data backup/restore output</b></summary>

```
$ rhai-cli migrate prepare --migration trustyai.data --target-version 3.5.0

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade
Backup directory: ./backup-migrate-20250708-153045

trustyai.data:

  → Discover TrustyAIService storage backends
    ✓ Found 1 PVC-backed instance, 1 MariaDB-backed instance
  → Backup PVC data for trustyai-service (trustyai-ns)
    ✓ Backed up PVC trustyai-pvc (2.3 GB)
  → Backup MariaDB for trustyai-service (ml-prod)
    ✓ Dumped database trustyai_db (450 MB)

Preparation trustyai.data completed successfully!

All preparations completed successfully!
Backups saved to: ./backup-migrate-20250708-153045

Run 'migrate run' to execute the migration.
```

</details>

---

## Kubeflow Training

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
# Verify training operator migration
chmod +x kubeflow-trainer-verification.sh
./kubeflow-trainer-verification.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration training.verify-workloads \
  --target-version 3.5.0
```

</td>
</tr>
</table>

<details>
<summary><b>Example: Training verification output</b></summary>

```
$ rhai-cli migrate run --migration training.verify-workloads --target-version 3.5.0

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

training.verify-workloads:

Running migration: training.verify-workloads

  → Discover Kubeflow v1 training workloads
    ✓ Found 2 PyTorchJobs, 0 TFJobs, 0 MPIJobs
  → Check PyTorchJob fine-tune-llama (team-ml)
    → Active — requires migration to Trainer v2 TrainJob before upgrade
  → Check PyTorchJob text-classifier (team-ds)
    ✓ Completed — safe to proceed

Migration training.verify-workloads completed with skipped steps
```

</details>

---

## LlamaStack

<table>
<tr><th>Bash Script</th><th>rhai-cli</th></tr>
<tr>
<td>

```bash
./backup-all-llamastack.sh
```

</td>
<td>

```bash
rhai-cli migrate run \
  --migration llamastack.backup \
  --target-version 3.5.0

# Or use prepare for explicit backup
rhai-cli migrate prepare \
  --migration llamastack.backup \
  --target-version 3.5.0 \
  --output-dir /backups
```

</td>
</tr>
</table>

<details>
<summary><b>Example: LlamaStack backup output</b></summary>

```
$ rhai-cli migrate prepare --migration llamastack.backup --target-version 3.5.0 --output-dir /backups

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade
Backup directory: /backups/backup-migrate-20250708-153045

llamastack.backup:

  → Discover LlamaStack resources
    ✓ Found 3 LlamaStack resources across 2 namespaces
  → Backup LlamaStack my-llama-stack (llm-serving)
    ✓ Saved llamastack-my-llama-stack-llm-serving.yaml
  → Backup LlamaStack prod-stack (ml-prod)
    ✓ Saved llamastack-prod-stack-ml-prod.yaml
  → Backup LlamaStack dev-stack (llm-serving)
    ✓ Saved llamastack-dev-stack-llm-serving.yaml

Preparation llamastack.backup completed successfully!

All preparations completed successfully!
Backups saved to: /backups/backup-migrate-20250708-153045

Run 'migrate run' to execute the migration.
```

</details>

---

## Kueue

This action has no bash script equivalent -- it is new in `rhai-cli`.

It migrates OpenShift AI built-in (embedded) Kueue to the Red Hat build of Kueue (RHBOK) operator: remove embedded Kueue, delete legacy v1alpha1 CRDs, install RHBOK via OLM, set DSC `managementState` to `Unmanaged`, label namespaces/workloads, and verify.

For as-built internals, see [rhbok-migration.md](rhbok-migration.md).

```bash
# Preflight + backup ClusterQueues and kueue-manager-config (when Managed)
rhai-cli migrate prepare \
  --migration kueue.rhbok.migrate \
  --target-version 3.5.0 \
  --output-dir /backups

# Run migration
rhai-cli migrate run \
  --migration kueue.rhbok.migrate \
  --target-version 3.5.0

# Dry-run
rhai-cli migrate run \
  --migration kueue.rhbok.migrate \
  --target-version 3.5.0 \
  --dry-run
```

**RHBOK-specific flags:**

| Flag | Default | Purpose |
|------|---------|---------|
| `--cluster-queue-name` | *(empty)* | Optional DSC `defaultClusterQueueName` |
| `--local-queue-name` | *(empty)* | Optional DSC `defaultLocalQueueName` |
| `--workload-queue-name` | `default` | Value for `kueue.x-k8s.io/queue-name` on workloads |
| `--channel` | resolved | OLM channel: existing Subscription → PackageManifest → fallback `stable-v1.2` |
| `--skip-remove-embedded` | `false` | Skip Managed → Removed (not recommended) |
| `--force-delete-legacy-crds` | `false` | Delete legacy cohorts/topologies CRDs even when instances exist (irreversible — also deletes existing Cohort/Topology instances; prepare does not back them up) |

<details>
<summary><b>Example: Kueue RHBOK migration output</b></summary>

```
$ rhai-cli migrate run --migration kueue.rhbok.migrate --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

kueue.rhbok.migrate:

Running migration: kueue.rhbok.migrate (confirmations skipped)

  → Verify RBAC permissions
  → Verify cert-manager is installed
  → Verify current Kueue state
  → Check for Red Hat build of Kueue operator conflicts
  → Resolve Red Hat build of Kueue operator channel
  → Verify Kueue resources exist
  → Report namespaces and workloads requiring labels
  → Preserve Kueue ConfigMap for reference
    ✓ Annotated ConfigMap opendatahub.io/managed=false
  → Uninstall embedded Kueue by setting managementState to Removed
    ✓ Set Kueue managementState to Removed
  → Wait for embedded Kueue to be removed
    ✓ Embedded Kueue removed
  → Delete legacy v1alpha1 Kueue CRDs
    ✓ Deleted cohorts.kueue.x-k8s.io
    ✓ Deleted topologies.kueue.x-k8s.io
  → Install Red Hat Build of Kueue Operator
    ✓ Created Namespace openshift-kueue-operator
    ✓ Created OperatorGroup
    ✓ Subscription created successfully
    ✓ ClusterServiceVersion ready
  → Activate Red Hat build of Kueue in DataScienceCluster
    ✓ Set Kueue managementState to Unmanaged
  → Wait for KueueReady condition
    ✓ KueueReady=True and operator pods ready
  → Apply kueue.openshift.io/managed=true to namespaces
  → Apply kueue.x-k8s.io/queue-name to workloads
  → Verify RHBOK migration completed successfully
    ✓ Migration verification passed
  → Check ClusterQueue and LocalQueue counts
    ✓ ClusterQueue and LocalQueue counts matched

Migration kueue.rhbok.migrate completed successfully!
```

</details>

---

## End-to-End Upgrade Workflow

### With Bash Scripts (Old Way)

Required cloning the repo, installing per-component dependencies, and running scripts manually in the correct order:

```bash
git clone https://github.com/red-hat-data-services/rhoai-upgrade-helpers.git
cd rhoai-upgrade-helpers

# --- PRE-UPGRADE ---

# 1. AI Pipelines pre-check
cd ai_pipelines && ./check_before_upgrade.sh && cd ..

# 2. Model Serving migrations
cd model-serving/before-upgrade
./serverless-to-raw.sh
./modelmesh-to-raw.sh --dry-run   # verify first
./modelmesh-to-raw.sh
./hardwareprofiles-ignorelist.sh
./add-owner-references.sh
cd ../..

# 3. TrustyAI backups
cd trustyai
./backup-metrics.sh
./backup-data.sh
./migrate-gorch-otel-exporter.sh --fix
cd ..

# 4. RayCluster backup (needs Python)
cd ray
pip install -r ray_cluster_migration_requirements.txt
python ray_cluster_migration.py pre-upgrade
cd ..

# 5. Workbench migration
cd workbenches
./workbench-2.x-to-3.x-upgrade.sh patch --all -y
cd ..

# 6. LlamaStack backup
cd llamastack && ./backup-all-llamastack.sh && cd ..

# === PERFORM RHOAI UPGRADE ===

# --- POST-UPGRADE ---

# 7. AI Pipelines post-check
cd ai_pipelines && ./post_upgrade_check.sh && cd ..

# 8. Model Serving post-upgrade
cd model-serving/after-upgrade
./managed-inferenceservice-config.sh
cd ../..

# 9. RayCluster migration
cd ray
python ray_cluster_migration.py post-upgrade --dry-run
python ray_cluster_migration.py post-upgrade
cd ..

# 10. TrustyAI post-upgrade
cd trustyai
./restore-metrics.sh
./restore-data.sh
./break-gpu-deadlock.sh --fix
./patch-guardrails-deployment.sh --fix
cd ..

# 11. Workbench cleanup and verify
cd workbenches
./workbench-2.x-to-3.x-upgrade.sh cleanup --all -y
./workbench-2.x-to-3.x-upgrade.sh verify --phase all --all
./workbench-2.x-to-3.x-upgrade.sh attach-kueue-label --all
cd ..

# 12. Training verification
cd kubeflow-trainer && ./kubeflow-trainer-verification.sh && cd ..
```

### With rhai-cli (New Way)

A single binary handles discovery, ordering, and execution:

```bash
# --- DISCOVER ---

# List all applicable migrations for your upgrade target
rhai-cli migrate list --target-version 3.5.0

# List only pre-upgrade actions
rhai-cli migrate list --target-version 3.5.0 --phase pre-upgrade

# List only post-upgrade actions
rhai-cli migrate list --target-version 3.5.0 --phase post-upgrade


# --- PRE-UPGRADE ---

# Option A: Run all pre-upgrade actions at once
rhai-cli migrate run \
  --phase pre-upgrade \
  --target-version 3.5.0 \
  --yes

# Option B: Run individual actions with dry-run first
rhai-cli migrate run \
  --migration ai-pipelines.pre-upgrade-check \
  --target-version 3.5.0 \
  --dry-run

rhai-cli migrate run \
  --migration ai-pipelines.pre-upgrade-check \
  --target-version 3.5.0


# === PERFORM RHOAI UPGRADE ===


# --- POST-UPGRADE ---

# Option A: Run all post-upgrade actions at once
rhai-cli migrate run \
  --phase post-upgrade \
  --target-version 3.5.0 \
  --yes

# Option B: Run individual actions
rhai-cli migrate run \
  --migration ai-pipelines.post-upgrade-check \
  --target-version 3.5.0

rhai-cli migrate run \
  --migration raycluster.migrate \
  --target-version 3.5.0

rhai-cli migrate run \
  --migration workbenches.verify-migration \
  --target-version 3.5.0
```

<details>
<summary><b>Example: Running all pre-upgrade actions at once</b></summary>

```
$ rhai-cli migrate run --phase pre-upgrade --target-version 3.5.0 --yes

Current OpenShift AI version: 2.25.0
Target OpenShift AI version: 3.5.0
Phase: pre-upgrade

=== Migration 1/9: kueue.rhbok.migrate ===

kueue.rhbok.migrate:

Running migration: kueue.rhbok.migrate (confirmations skipped)

  → Preserve Kueue ConfigMap for reference
  → Uninstall embedded Kueue by setting managementState to Removed
  → Wait for embedded Kueue to be removed
  → Delete legacy v1alpha1 Kueue CRDs
  → Install Red Hat Build of Kueue Operator
    ✓ Subscription created successfully
    ✓ ClusterServiceVersion ready
  → Activate Red Hat build of Kueue in DataScienceCluster
    ✓ Set Kueue managementState to Unmanaged
  → Wait for KueueReady condition
  → Apply kueue.openshift.io/managed=true to namespaces
  → Apply kueue.x-k8s.io/queue-name to workloads
  → Verify RHBOK migration completed successfully
  → Check ClusterQueue and LocalQueue counts

Migration kueue.rhbok.migrate completed successfully!

=== Migration 2/9: ai-pipelines.pre-upgrade-check ===

ai-pipelines.pre-upgrade-check:

Running migration: ai-pipelines.pre-upgrade-check (confirmations skipped)

  → Capture DSPA pod health baseline
    ✓ Saved pod health state for 4 DSPAs across 3 namespaces
  → Migrate v1alpha1 DSPAs to v1
    ✓ Migrated 2 DSPAs from v1alpha1 to v1
  → Detect custom RBAC roles
    ✓ No custom roles require updating

Migration ai-pipelines.pre-upgrade-check completed successfully!

=== Migration 3/9: modelserving.serverless-to-raw ===

  ...

=== Migration 9/9: llamastack.backup ===

llamastack.backup:

Running migration: llamastack.backup (confirmations skipped)

  → Discover LlamaStack resources
    ✓ Found 3 LlamaStack resources across 2 namespaces
  → Backup LlamaStack resources
    ✓ Saved 3 backup files

Migration llamastack.backup completed successfully!

All migrations completed successfully!
```

</details>

### Using the Container for End-to-End

Same container invocation as [Getting Started](#getting-started) (pin a `:vX.Y.Z` tag so both steps run the same binary), applied to the full pre/post-upgrade workflow:

```bash
# Pre-upgrade: run all pre-upgrade migrations
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:ro \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate run --phase pre-upgrade --target-version 3.5.0 --yes

# Post-upgrade: run all post-upgrade migrations
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:ro \
  quay.io/rhoai/rhai-cli-rhel9:v3.5.0 \
  migrate run --phase post-upgrade --target-version 3.5.0 --yes
```

---

## Common Flags Reference

### Global Flags

| Flag                                | Description                                                                                                                            |
|-------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|
| `--target-version X.Y.Z`            | Required. The RHOAI version you are upgrading to.                                                                                      |
| `--migration <id>`                  | Run a specific migration action by ID.                                                                                                 |
| `--phase pre-upgrade\|post-upgrade` | Filter actions by upgrade phase.                                                                                                       |
| `--dry-run`                         | Preview changes without applying them.                                                                                                 |
| `--yes`                             | Skip all confirmation prompts (for CI/automation).                                                                                     |
| `--output-dir <path>`               | Custom output directory for backups (`migrate prepare`).                                                                               |
| `--verbose`                         | Enable detailed output. Always on for `migrate run` and `migrate prepare` (the flag has no effect there); optional for `migrate list`. |

### Workbench Flags

These flags are shared across all workbench actions (`patch-auth-model`, `cleanup-oauth`, `verify-migration`, `attach-kueue-label`) but **not** `upgrade-2x-to-3x`, which always operates cluster-wide:

| Flag                         | Description                                                          |
|------------------------------|----------------------------------------------------------------------|
| `--workbench-namespace <ns>` | Limit to notebooks in this namespace (default: all namespaces).      |
| `--workbench-name <name>`    | Target a single notebook by name (requires `--workbench-namespace`). |

**`patch-auth-model` only:**

| Flag             | Description                                                                               |
|------------------|-------------------------------------------------------------------------------------------|
| `--skip-stop`    | Skip auto stop/restart of running workbenches before patching.                            |
| `--only-stopped` | Only patch stopped workbenches, skip running ones. Mutually exclusive with `--skip-stop`. |
| `--with-cleanup` | Chain OAuth cleanup after patching (delete legacy Route, Service, Secrets, OAuthClient).  |

**`verify-migration` only:**

| Flag                     | Description                                                 |
|--------------------------|-------------------------------------------------------------|
| `--verify-phase <phase>` | What to verify: `migration` (default), `cleanup`, or `all`. |

**`upgrade-2x-to-3x` only:**

| Flag                  | Description                                                                 |
|-----------------------|-----------------------------------------------------------------------------|
| `--force-non-stopped` | Include non-stopped (running) workbenches. **Unsafe: may cause data loss.** |

### RayCluster Flags

| Flag                              | Description                                                                                              |
|-----------------------------------|----------------------------------------------------------------------------------------------------------|
| `--raycluster-namespace <ns>`     | Limit to RayClusters in this namespace (default: all namespaces).                                        |
| `--raycluster-cluster <name>`     | Target a single RayCluster by name (requires `--raycluster-namespace`).                                  |
| `--raycluster-output-dir <path>`  | Directory for RayCluster backup YAML files (`raycluster.backup` only).                                   |
| `--raycluster-from-backup <path>` | Restore from a backup file or directory; deletes the existing cluster first (`raycluster.migrate` only). |
| `--raycluster-timeout <duration>` | Timeout waiting for the cluster route to become available (`raycluster.migrate` only).                   |

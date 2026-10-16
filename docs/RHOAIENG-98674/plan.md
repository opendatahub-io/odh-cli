# RHOAIENG-98674 — DSC v1 storage validation and migration

| Field | Value |
|---|---|
| Status | in progress |
| Start date | 2026-10-05 |
| End date | TBD — record the actual date when all acceptance criteria are met |
| Last updated | 2026-10-05 |
| Parent issue | [RHOAIENG-98674](https://issues.redhat.com/browse/RHOAIENG-98674) |
| Epic | [RHOAIENG-94804](https://issues.redhat.com/browse/RHOAIENG-94804) |
| Team / component | AI Core Platform |
| Assignee | Luca Burgazzoli (inherited by the subtasks) |
| Release | 3.6 GA RHOAI RELEASE |

## Authority and how to resume

This file and the task files in tasks/ are the complete working specification for this work. They contain the Jira scope, decisions from this session, repository findings, test expectations, and acceptance criteria. An implementation agent should not need to open Jira to understand or complete the work. Jira links below are for traceability only.

This plan is authoritative if Jira wording is incomplete or conflicts with the technical decisions recorded here. Do not change Jira as part of implementation. If implementation uncovers a necessary deviation, write the evidence, chosen behavior, and impact in the relevant task file and in this plan before proceeding.

Each task uses only these statuses: new, in progress, done. Record actual implementation start and completion dates; leave the end date TBD while work is in progress. On the first implementation change, change status to in progress and set the actual start date. On completion, set status to done and record the end date. Keep the dates in ISO format (YYYY-MM-DD).

At each status change, append a dated progress note to the task file. When a task is done, add its implementation notes, test commands and results, unresolved limitations, and development/testing learnings to that task file. Copy generalizable learnings into this plan so the plan remains the authoritative record. Keep the task file and this plan synchronized; do not rely on Jira comments for progress.

### Resume checkpoint — 2026-10-05

RHOAIENG-98680 is done. RHOAIENG-98681 and RHOAIENG-98682 are implemented and every gate passes locally: go test for both packages, make fmt, make lint/fix (also with GOFLAGS=-tags=integration), make check, make test, and make test/integration (three Kind tests, about 123s). They stay in progress until two points are settled: the CI storage-migration job has not run yet, and the cause of the stalled second migration on a reused cluster (see the learning log) is unknown. Code: pkg/migrate/actions/datasciencecluster (storage_version.go, storage_version_task.go, storage_version_migration.go, storage_version_test.go) and tests/integration/storage-migration (storage_migration_test.go, cluster_test.go, cluster_kind_test.go, cluster_external_test.go, cli_test.go). The suite also runs against an existing cluster with STORAGE_MIGRATION_CLUSTER=external; that mode deletes the cluster's DataScienceCluster CRD.

Resume in this order:

1. Read this plan, all three task files, and the required repository guidance in AGENTS.md. Inspect the working tree and any existing implementation before changing files; do not assume a clean checkout.
2. Implement RHOAIENG-98680 first, following the DSC platform checks in pkg/lint/checks/platform/datasciencecluster. Register platform.dsc.storage-version explicitly.
3. Implement RHOAIENG-98681 as the dsc.storage-version.migrate action in the existing migration framework, following neighboring actions and pkg/migrate/registry.go. Keep the existing migrate run command and its target-version requirement.
4. Implement RHOAIENG-98682 with one Kind harness and a CRD fixture built in Go. Run v1-only and v1-to-v2 in one Kind cluster, then run v2-to-v3 in a fresh Kind cluster with v2 data. Verify safe failure separately with the API disabled.
5. After each task, record exact code locations, test/check commands and outcomes, remaining work, and the next resume step in that task and this checkpoint. Do not mark a task done until its acceptance criteria pass.

Preserve the decisions below when resuming:

- Invocation: odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0. No new direct subcommand is required. This user decision supersedes the initial direct-command proposal derived from Jira wording.
- Lint applicability: any source version to a target at least 3.6, without an upper-major-version restriction. Use existing version helpers and preserve the lint command's existing execution flow.
- Target-version is retained for the generic runner's selection/phase behavior. It does not influence the DSC task's destination or rewrite logic: the actual CRD storage version is authoritative.
- Integrations use Kind only. Use the real migration controller for upgrades and confirmed API absence for a safe failure test; keep schema/error permutations in ordinary Go unit tests.
- CRD progression: v1 only (v1 storage), then v1 + v2 (v2 storage), then v2 + v3 (v3 storage). Perform migration before removing a version still recorded in storedVersions.
- Build the minimal CRD fixture in Go with an open schema. Do not use an embedded template. This user decision supersedes the earlier embedded-template choice.
- Run the lint and migrate commands in-process through their cobra wiring; do not build or spawn the binary. Do not read etcd: a rewrite is shown by the object's resourceVersion changing.
- Code style agreed in review: one statement, field or parameter per line; switch instead of if/else chains; no one-line wrapper functions; names that say what the thing does. Keep tests simple and fake-client only at unit level.
- Follow existing checks, action/task, recorder, dry-run, confirmation, output, and timeout conventions. Keep helpers focused on DSC and avoid broad framework/client refactors.

## Goal and upgrade context

Add two capabilities to odh-cli for retiring the DataScienceCluster v1 storage version before the RHOAI 3.6 DSC v1 API removal:

1. A blocking upgrade-readiness check in the existing lint target-version flow.
2. A registered migration action, dsc.storage-version.migrate, executed through odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0. It discovers and uses a supported StorageVersionMigration API and controller. The CLI never updates DSC custom resources directly.

Add Kind integration coverage for the validation-to-migration flow.

For this release, DataScienceCluster v3 is the storage version and v2 remains served through conversion. The concern is whether v1 is still recorded as a stored version in the DataScienceCluster CRD. Reading a DataScienceCluster through the v1 endpoint does not answer that question: the apiserver can convert the stored object to the requested version during the read. The check must inspect the CRD's status.storedVersions directly. It must not fetch or list DataScienceCluster objects through v1 to infer their etcd encoding.

The release context does not hardcode the migration destination to v3. The task follows whichever version the installed CRD currently marks storage, including v2 in an intermediate upgrade state. A v1-only CRD whose storedVersions is [v1] is a migration no-op and remains blocked by the v1 lint check; the action does not introduce a newer version or change spec.versions.

The relevant CRD identity is:
- Group: datasciencecluster.opendatahub.io
- Plural: datascienceclusters
- Kind: DataScienceCluster
- CRD name: datascienceclusters.datasciencecluster.opendatahub.io
- Scope: Cluster

status.storedVersions records versions that have been used as storage versions. The apiserver appends storage versions; ordinary object rewrites do not prune this history. A migration controller may handle the status cleanup, otherwise the action must do so after confirmed rewrites. The CRD version cannot safely be dropped while an old version remains in status.storedVersions; OLM also blocks such an upgrade.

## Current repository findings

These findings were checked in the target odh-cli checkout on 2026-10-05:

- The migration Cobra group is in cmd/migrate/migrate.go. It currently registers list, prepare, and run. Migration implementations are registered through pkg/migrate/registry.go.
- The initial Jira wording proposed a direct subcommand. The user explicitly superseded that proposal: register dsc.storage-version.migrate and use the existing migrate list/run command surfaces.
- Existing stored-version handling for DataSciencePipelinesApplication is in pkg/migrate/actions/aipipelines/dspa.go, with tests in dspa_test.go and orchestration in precheck.go. Reuse sound patterns and avoid a second, inconsistent CRD helper where practical.
- The AIPipelines helper treats a missing CRD as no migration and suppresses some permission failures. DSC must surface read/permission failures and use a fresh CRD resourceVersion for status pruning.
- Lint checks are explicitly registered in pkg/lint/command.go. The lint Target client is read-only. A lint check that reports a blocking requirement failure must explicitly set ImpactBlocking; the default impact for a failed condition is advisory.
- Shared GetResource can suppress permission errors by returning nil, and List/ListMetadata can suppress them as empty lists. Their WithLimit behavior can also stop after a total item limit. The lint check must reject nil CRD reads; migration must use strict low-level CRD and migration-request operations. Do not change those helpers globally as part of this task.
- The existing migrate run runner requires target-version and performs cluster-version/phase detection. Keep that interface; the DSC task itself ignores the target release and uses the CRD storage version. Shared migration options already provide a 10-minute timeout, dry-run, confirmation flags, and structured output.
- The current Makefile's make check target runs lint only. Completion also requires make test and the new make test/integration target.
- The requested docs/RHOAIENG-98674 directory did not exist when this plan was started, and the odh-cli working tree was clean.

## Required behavior and safety contract

### Validation

- Add platform.dsc.storage-version alongside the existing DSC platform checks, using BaseCheck, existing condition constructors, and version annotations.
- CanApply uses existing version helpers to apply for any source version when the target is at least 3.6, including later major releases. A missing target or a target below 3.6 is not applicable; do not impose a source-version restriction.
- Read the DataScienceCluster CRD and inspect status.storedVersions.
- If v1 appears anywhere in storedVersions, return a failing diagnostic with explicit blocking impact and clear remediation: run odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0 before dropping v1.
- If v1 is absent, return a passing diagnostic.
- Do not read or list DSC custom resources at the v1 API version. API conversion makes that an invalid test of the stored representation.
- A missing CRD, missing/malformed status, forbidden access, nil CRD read, or API read failure returns an execution error; never treat uncertainty as a clean pass.
- Register the check explicitly. Follow the current lint check package, result, and condition conventions.
- Do not use DSC-fetching builders for this CRD-only check.

### Migration API discovery

Discover APIs served by the actual cluster at runtime. Jira identifies Kubernetes StorageVersionMigration APIs in storagemigration.k8s.io and the OpenShift migration.k8s.io/v1alpha1 API as candidates. The user-provided design also lists Kubernetes storagemigration.k8s.io/v1, v1beta1, and v1alpha1 as possible served versions.

Do not choose an API based only on a Kubernetes or OpenShift release-number matrix. The design notes supplied in the session explicitly say that some feature-gate defaults and the OpenShift version matrix were not verified. Discovery is authoritative: determine which group/version/resource is served and create a request with the correct schema for that API. Older and newer APIs can use different spec shapes.

Select deterministically from served, supported APIs: Kubernetes v1, then v1beta1, then v1alpha1, then OpenShift migration.k8s.io/v1alpha1. Keep request constructors small and verify each supported schema against its actual API definition; do not introduce generic schema reflection or a release matrix.

If no supported migration API is served, fail with instructions to enable the cluster storage migration API and controller. Discovery, authorization, and connection errors also fail. The CLI does not list or patch DSC objects to migrate their storage.

As built, the action asks discovery about each candidate group/version in turn and treats NotFound as "not served". It does not list every API on the cluster, so an unrelated unhealthy aggregated API cannot fail the migration.

A served API alone does not prove that a controller is reconciling requests. The command must fail clearly on timeout or failed status; Kind tests provide a functioning controller for successful migrations and verify unchanged objects/history when the API is unavailable.

### Migration flow

The user confirmed this tool runs once on a stable cluster before the upgrade. Use a fixed request name; do not add UID/generation tracking, automatic restart machinery, or CRD-race test matrices.

1. Read the DataScienceCluster CRD and its current storage version. If status.storedVersions is already exactly the current storage version, report a no-op and exit successfully.
2. If discovery finds a supported migration API, create its request with the fixed name dsc-storage-migration or resume that request if it already exists. Poll for Succeeded=True. Fail on Failed=True, timeout, cancellation, permission error, or API error.
3. After successful rewrites, re-read the CRD and prune storedVersions to its current storage version using the fresh resourceVersion. Surface a conflict or unexpected storage-version change as an error; do not automatically restart.
4. Delete the completed request after successful handling. Retain failed or interrupted requests for diagnosis and resumption. Failed requests require review and explicit deletion before retry. Surface cleanup errors.
5. Never trim status.storedVersions after a failed, timed-out, incomplete, or otherwise unconfirmed rewrite.

Every request carries the annotation odh-cli.opendatahub.io/storage-version with the storage version it was created for. An existing request is reused only when that annotation matches the current storage version; otherwise the action fails and asks the user to review and delete it. This closes the earlier gap where a request that had already succeeded for a previous storage version could lead to pruning without a rewrite (raised in PR review).

### Migration controller requirement

User correction on 2026-10-05 supersedes the earlier fallback plan: remove rewriteObjects and all direct DSC updates. Migration requires a supported StorageVersionMigration API and a functioning controller. The controller performs the storage rewrites; the CLI only manages the request and updates CRD storedVersions after Succeeded=True. No supported API means an actionable failure with unchanged DSC objects and CRD history.

### Permissions and operational behavior

Document and verify the required permissions for:
- Reading the DataScienceCluster CRD.
- Creating, getting, and deleting the discovered migration resource (the command polls; it does not require watch).
- Updating customresourcedefinitions/status after a successful rewrite.

Return actionable errors for API discovery, permissions, migration failure, timeout, missing API/controller, status update, and cleanup. Do not claim success or clear v1 from status after a partial migration.

Implement as a normal pre-upgrade action with no separate prepare task. Reuse the action/task interfaces, existing step recording and confirmation, dry-run behavior, output handling, and default 10-minute timeout. Dry-run and declined confirmation perform no writes. Keep resource identities centralized and new helpers local to DSC; avoid a broad framework or shared-client refactor.

### Integration contract

Kind is the sole integration harness. Use one Go Kind lifecycle helper, with cleanup registered immediately and bounded waits and failure logs. Pin sigs.k8s.io/kind v0.31.0 and kindest/node:v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f from the [published release](https://github.com/kubernetes-sigs/kind/releases/tag/v0.31.0).

Run the actual v1-only and v1-to-v2 states in one Kind cluster, then run v2-to-v3 from v2-stored data in a fresh Kind cluster. The migration action is a one-off pre-upgrade command for each path. With migration APIs disabled, verify safe failure at v1/v2 without any object or history change. Exercise the real lint and migrate commands in-process (a cobra root wired like cmd/main.go, with the cluster's kubeconfig), ordinary version detection, data preservation, failure safety, and idempotent reruns. make test/integration runs the suite and does not depend on make build; CI runs it as a single required job, and unavailable prerequisites must fail that job.

Build the DSC CRD fixture in Go with the production identity and an open schema (x-kubernetes-preserve-unknown-fields) in every version, so no conversion webhook is needed. The test object carries spec.marker and status.release.version. No production DSC spec is needed for this fixture.

| Stage | CRD versions / storage | Required outcome |
|---|---|---|
| Initial | v1 only / v1 storage | Create v1-stored data. Lint targeting at least 3.6 blocks. Migration is a no-op retaining [v1], despite target-version 3.6.0. |
| Intermediate | v1 + v2 / v2 storage | History contains v1/v2 and existing data remains v1. Lint blocks. Migration rewrites to v2, preserves data, leaves [v2], and clears the finding. |
| Final | v2 + v3 / v3 storage | After successful v2 cleanup, remove v1 and introduce v3 storage. History contains v2/v3; lint already passes. Migration rewrites to v3 and leaves [v3]. |

The suite does not read etcd. A rewrite is shown by the object's resourceVersion changing across the migration, and "nothing was written" by it staying the same. Assert that removing v1 before migration/history cleanup is rejected and that permission failures do not clear old versions. Use ordinary Go unit tests for API-schema permutations and detailed error cases; do not add envtest or a custom migration controller.

## Task map and sequence

| Order | Task file | Jira | Task | Dependencies |
|---|---|---|---|---|
| 1 | [DSC v1 stored-version validation](tasks/dsc-v1-stored-version-validation.md) | [RHOAIENG-98680](https://issues.redhat.com/browse/RHOAIENG-98680) | Add blocking CRD status validation and unit tests. | Implement first. |
| 2 | [DSC storage-version migration action and upgrade tests](tasks/dsc-storage-version-migration-command.md) | [RHOAIENG-98681](https://issues.redhat.com/browse/RHOAIENG-98681) | Register the action; add discovery, migration requests, status cleanup, and unit/upgrade-flow tests. | Implement after task 1. |
| 3 | [Kind integration tests](tasks/kind-integration-tests.md) | [RHOAIENG-98682](https://issues.redhat.com/browse/RHOAIENG-98682) | Exercise the actual CRD stages with the real controller and safe failure on API absence using a minimal Go-built CRD fixture. | Implement after tasks 1 and 2. |

## Cross-task acceptance criteria

- The lint check blocks only when v1 is recorded in CRD status.storedVersions and does not infer storage state from v1 API reads.
- The check applies for any source version when the target is at least 3.6; valid targets below 3.6 do not select it.
- The action is registered as dsc.storage-version.migrate and remediation uses odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0.
- Target-version remains a runner requirement and does not influence the task's destination or rewrite behavior; the actual CRD storage version is authoritative.
- The command requires an actually served migration API and controller; missing APIs fail without updating DSC objects or clearing history.
- Failed or incomplete migrations never trim status.storedVersions.
- Migration runs on a stable cluster before the upgrade; status is re-read before pruning and write failures are surfaced.
- Status cleanup uses optimistic concurrency and preserves the current storage version.
- Kind test fixtures use the production CRD identity but only a deliberately minimal schema. No full DSC spec is required.
- The real-controller Kind configuration exercises v1-only, v1 + v2 storage, and v2 + v3 storage, showing each rewrite by a changed resourceVersion. The unavailable-API configuration verifies failure without writes. The final v2/v3 stage passes the v1 lint check even before its v2-to-v3 migration.
- The CRD fixture is built in Go; CI runs all three integration tests without silent skips.
- All three task files contain completion evidence, exact test commands/results, and task-specific learnings. Generalizable learning is copied into this plan.
- Focused tests, make fmt, make lint/fix, make check, make test, and make test/integration pass before code work is marked done. Record exact commands/results; make check alone runs only lint, and it skips the integration suite unless run with GOFLAGS=-tags=integration.

## Related JIRAs

### Direct hierarchy

- [RHOAIENG-94804 — DSC v3 introduction epic](https://issues.redhat.com/browse/RHOAIENG-94804)
- [RHOAIENG-98674 — Add DSC v1 storage validation and migration to odh-cli](https://issues.redhat.com/browse/RHOAIENG-98674)
- [RHOAIENG-98680 — Add DSC v1 stored-version validation and unit tests](https://issues.redhat.com/browse/RHOAIENG-98680)
- [RHOAIENG-98681 — Implement DSC storage-version migration command and upgrade-flow unit tests](https://issues.redhat.com/browse/RHOAIENG-98681)
- [RHOAIENG-98682 — Add Kind integration tests for DSC storage-version migration](https://issues.redhat.com/browse/RHOAIENG-98682)

The parent and all three subtasks were New when this plan was captured on 2026-10-05. The subtasks inherit AI Core Platform and Luca Burgazzoli from the parent, and target 3.6 GA RHOAI RELEASE.

### DSC v3 context and upstream work

- [RHOAIENG-85262 — Evaluate need for a DataScienceCluster v3 API](https://issues.redhat.com/browse/RHOAIENG-85262) — decision context referenced by the epic.
- [RHOAIENG-94809 — Finalize the public API and conversion contract](https://issues.redhat.com/browse/RHOAIENG-94809) — version/conversion context; use the current v3 storage and v2 conversion contract.
- [RHOAIENG-94812 — Implement DSC v3 API](https://issues.redhat.com/browse/RHOAIENG-94812) — v3 storage API implementation.
- [RHOAIENG-94813 — Verify upgrade and compatibility](https://issues.redhat.com/browse/RHOAIENG-94813) — includes storage-version migration verification.
- [RHOAIENG-94814 — Identify scope of work for DSC v1 retirement](https://issues.redhat.com/browse/RHOAIENG-94814) — defines safe v1 retirement.

Session correction: the documentation issues RHOAIENG-98660 and RHOAIENG-89352 are not a related pair. They are not dependencies or inputs to this CLI plan.

## Repository development and testing guidance

This plan does not replace the odh-cli repository instructions. Relevant local guidance:
- docs/development.md and docs/design.md for the CLI architecture.
- docs/lint/architecture.md and docs/lint/writing-checks.md for lint check registration, applicability, blocking impact, results, and tests.
- docs/migrate/rhbok-migration.md and docs/extensibility.md for current migration command and action patterns.
- docs/testing.md, docs/quality.md, docs/setup.md, and docs/coding/ for test, code, formatting, and quality rules.

Key constraints for this work:
- Lint checks implement the existing check contract, use explicit registration, and return errors for infrastructure/read failures. A failed condition is advisory by default; explicitly use blocking impact for this required upgrade stop.
- Use centralized resource definitions/helpers from pkg/resources where available. Check existing definitions before introducing group/resource strings.
- Current lint Target.Client is read-only. Do not add migration writes to the lint check.
- Current migrate group is action-based. Register the new action and reuse migrate list/run without adding a direct subcommand.
- Follow package test conventions: vanilla Gomega, t.Run subtests, t.Context(), package-level test fixture constants, and fake clients/test helpers as appropriate.
- Use make fmt for formatting, make lint/fix before manual lint fixes, and make check after implementation. Direct targeted go test is allowed by the repository guide; record the exact command and result.
- Run focused tests and quality checks after each task; finish with make test and make test/integration as well. Do not claim planned tests have run or set implementation dates for planning-only edits.
- For Kind, use the Go Kind library to own cluster lifecycle; do not shell out to an installed kind binary as a replacement.

## Development and testing learnings

### Established before implementation

- A successful read of a custom resource through an older served API is not evidence of its on-disk storage version. The decisive validation input is the CRD's status.storedVersions.
- The new lint check must not perform writes; the migration controller rewrites objects, and the action updates CRD status only after success.
- A migration API's presence and its controller's health are distinct. Discovery determines API availability; status polling/timeout determines whether migration actually completed.
- Migration requests confirm object rewrites; re-read CRD status before pruning storage history.
- Kubernetes migration API versions and OpenShift API behavior must be selected from live discovery, not a release-version table. The supplied matrix has unverified feature-gate and OpenShift details.
- Existing AIPipelines stored-version helpers provide useful examples and fixtures, but DSC must use strict permission handling and a fresh status read.
- The user chose the current migrate list/run action interface over the initial direct-command proposal. This is resolved: do not add a second command surface when resuming.
- A valid v1-only CRD cannot be made v1-free by a storage rewrite while v1 remains its storage version. A no-op action can therefore coexist with a blocking v1 lint result until the CRD is upgraded.
- v2-to-v3 storage migration and v1 retirement validation are distinct: the former may still be needed when the latter already passes.

### Learning log

| Date | Learning or decision |
|---|---|
| 2026-10-05 | Plan created from Jira MCP issue details, session decisions, and the target repo's AGENTS.md and docs. Implementation has not started; no implementation tests have run. |
| 2026-10-05 | Confirmed the parent is under epic RHOAIENG-94804; child tasks are RHOAIENG-98680, RHOAIENG-98681, and RHOAIENG-98682, all New. |
| 2026-10-05 | Found existing storedVersions code in pkg/migrate/actions/aipipelines/dspa.go; its nil/permission handling and status update do not meet this task's fail-closed and compare-and-swap requirements. |
| 2026-10-05 | Initial plan proposed a direct command based on Jira wording. Superseded by the user decision below to use the existing migration action surface. |
| 2026-10-05 | User chose migrate run --migration dsc.storage-version.migrate --target-version 3.6.0. Keep the runner flag; derive the task destination only from the current CRD storage version. |
| 2026-10-05 | User selected lint applicability from any source to targets at least 3.6, including later major releases, with no source-version gate. |
| 2026-10-05 | User selected Kind for all integrations, actual v1-only → v1/v2 storage → v2/v3 storage progression, and embedded in-memory Go templates for minimal CRD creation. |
| 2026-10-05 | Synchronized the parent and task specifications and added a resume checklist. All tasks remain new; no code implementation or implementation tests have started. |
| 2026-10-05 | User removed the in-process rewrite fallback: require a migration CR/controller and do not update user DSC objects from the CLI. Unit tests use fake clients directly, with focused failure tests. CRD history cleanup uses the typed UpdateStatus client. |
| 2026-10-05 | Review for readability. Action code restructured into a storageVersionMigration struct (run, servedAPI, newRequest, waitForRequest, pruneStoredVersions) and files renamed to storage_version_task.go and storage_version_migration.go. Discovery now asks per candidate group/version instead of listing every API. |
| 2026-10-05 | Unit tests consolidated from four files into one, storage_version_test.go: one fixture struct holding the typed fakes, table-driven cases, and a create reactor on the fake dynamic client standing in for the migration controller. |
| 2026-10-05 | Integration suite simplified at the user's direction: commands run in-process instead of spawning the built binary; the CRD is built in Go instead of from an embedded template; the etcd read was removed in favour of comparing resourceVersion; scenarios became three plain test functions and the CI matrix a single job; helper files merged down to three. |
| 2026-10-05 | cobra subcommands in this repo capture the root command's streams when they are added, so an in-process test must set the root's in/out/err before calling AddCommand, and build a new root per invocation so flag values do not leak. |
| 2026-10-05 | Re-confirmed the stalled second migration: on one cluster, after v1-to-v2, the v2-to-v3 request stayed Running for a full 4 minutes (normally seconds). The controller-manager log at default verbosity shows nothing useful. Cause still unknown; it may matter for real clusters that migrate the same CRD twice. |
| 2026-10-05 | Linting the integration suite needs GOFLAGS=-tags=integration; plain make check skips it. Doing so caught one unwrapped error in the suite. |
| 2026-10-05 | Integration suite split into cluster and cli structs and given an external-cluster mode (STORAGE_MIGRATION_CLUSTER=external). At the user's direction it wipes the DataScienceCluster CRD before and after each test rather than refusing to run when one exists. t.Context() is already cancelled inside t.Cleanup, so the wipe uses its own context. |
| 2026-10-05 | On one long-lived cluster, v1-to-v2 followed by v2-to-v3 completes when the CRD is deleted and recreated in between. The earlier stall therefore depends on carrying the same CRD through both steps, which is what a real upgrade does. Still unexplained. |

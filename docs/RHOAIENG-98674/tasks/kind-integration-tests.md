# Task — Kind integration tests for DSC storage-version migration

| Field | Value |
|---|---|
| Jira | [RHOAIENG-98682](https://issues.redhat.com/browse/RHOAIENG-98682) |
| Parent | [RHOAIENG-98674](https://issues.redhat.com/browse/RHOAIENG-98674), under [RHOAIENG-94804](https://issues.redhat.com/browse/RHOAIENG-94804) |
| Status | in progress |
| Start date | 2026-10-05 |
| End date | TBD — set when acceptance criteria pass |
| Last updated | 2026-10-05 |
| Assignee / team | Luca Burgazzoli / AI Core Platform |
| Release | 3.6 GA RHOAI RELEASE |

## Objective

Add end-to-end integration coverage for the DSC v1 stored-version check and the registered dsc.storage-version.migrate action. Use the Go Kind library to create and tear down an ephemeral cluster; follow the supplied [Kind helper pattern](https://github.com/lburgazzoli/opendatahub-module-operator/tree/db-service/modules/opendatahub-db-operator/test/support/cluster/kind).

Kind is the only integration harness for this work. Use ordinary Go unit tests for API-schema permutations and detailed errors. The integration tests run the real lint and migrate commands in-process against a real API server, through the actual CRD upgrade stages, using the real migration controller. Do not add envtest or a custom migration controller, and do not silently skip any test in CI.

The user-facing invocation is:

    odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0 --yes

The --yes flag avoids interactive prompts in the integration test. Keep --target-version for the existing runner, but verify that the migration task's destination is the CRD's current storage version rather than the supplied release.

## Minimal mock DataScienceCluster CRD

A deliberately minimal mock CRD is sufficient. It must use the production resource identity so odh-cli discovery and lookup exercise the real path:

- CRD name: datascienceclusters.datasciencecluster.opendatahub.io
- Group: datasciencecluster.opendatahub.io
- Plural: datascienceclusters
- Kind: DataScienceCluster
- Scope: Cluster
- Versions: only v1 initially; v1 and v2 with v2 storage in the intermediate state; v2 and v3 with v3 storage in the final state. Expose only the versions present at each stage.
- Schema: the same open schema (type object with x-kubernetes-preserve-unknown-fields) in every version. The test object carries spec.marker for data preservation and status.release.version for the CLI's existing version detection. Use a source release below 3.6 so explicit target-version lint executes in the normal upgrade flow.

The mock does not need the production DataScienceCluster spec, production controllers, module CRs, or a conversion webhook. The API scenario uses the real upstream storage migration controller supplied by the cluster. Each one-off upgrade path runs in a fresh cluster.

### CRD fixture

Superseded by user decision on 2026-10-05: there is no embedded template. newCRD in cluster_test.go builds the CustomResourceDefinition as a Go struct from a list of version names; the last one is the storage version. installCRD creates or updates it and waits until every version can be listed.

### Kind configuration

Use one focused Go Kind lifecycle helper, pin sigs.k8s.io/kind v0.31.0, and use kindest/node:v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f from the [published release](https://github.com/kubernetes-sigs/kind/releases/tag/v0.31.0). The cluster lifecycle is owned by the Go library, not a shell invocation of kind.

- API scenario: enable the supported Kubernetes migration API and its real upstream controller using the required feature gates/runtime configuration. Confirm discovery and actual request reconciliation; merely serving the API is insufficient.
- Unavailable scenario: disable migration APIs, confirm absence through discovery, and assert that migration fails at v1/v2 without changing objects or history.
- Use isolated ephemeral clusters for these configurations, bounded startup/migration waits, and cleanup registered immediately. Capture enough API/controller/cluster logs on failure to diagnose CI.

## End-to-end flow

As built, three test functions in storage_migration_test.go. Each creates its own cluster and reads top to bottom, with a t.Log line before each phase. lint is run as lint --checks platform.dsc.storage-version --target-version 3.6.0 --output json, and migrate as migrate run --migration dsc.storage-version.migrate --target-version 3.6.0 --timeout 90s --output json --yes.

TestMigrateV1ToV2 (migration API enabled):

1. Install the CRD with v1 only and create the DSC. History is [v1] and lint blocks. Migration succeeds as a no-op; history is still [v1] and lint still blocks.
2. Install v1 + v2 with v2 storage. History is [v1, v2], lint blocks, and an update that removes v1 from the CRD is rejected.
3. Run the migration as a user who can only read the CRD and the DSCs: it fails with "forbidden". Run it with --dry-run: it succeeds. After both, history and the object's resourceVersion are unchanged.
4. Run the migration. The resourceVersion changes, history is [v2], spec.marker and status.release.version are intact through both v1 and v2, and lint passes. Running again leaves the resourceVersion unchanged.
5. Remove v1 from the CRD; this now succeeds.

TestMigrateV2ToV3 (migration API enabled, fresh cluster):

1. Install v2 only, create the DSC, then install v2 + v3 with v3 storage. History is [v2, v3] and lint already passes.
2. Run the migration. The resourceVersion changes, history is [v3], the data is intact through v2 and v3, and lint passes. Running again leaves the resourceVersion unchanged.

TestMigrateWithoutMigrationAPI (migration API disabled):

1. Install v1, create the DSC, install v1 + v2.
2. Run the migration: it fails with the "enable the cluster storage migration API and controller" message. The resourceVersion and history are unchanged and lint still blocks.

Keep the progression faithful: no v1-to-v3 shortcut, and never all three versions at once.

## Migration mechanism coverage

- When testing a StorageVersionMigration API, the selected Kind configuration must serve the discovered API and run a functioning controller that reconciles the request. API discovery without a controller is not a successful test setup.
- With APIs disabled, assert an actionable failure and unchanged DSC objects/history. There is no direct-object rewrite fallback.
- Both the real-controller upgrade path and unavailable-API failure scenario are required in CI. Do not skip either because prerequisites or a controller are unavailable; fail the integration job with actionable setup diagnostics.
- Keep API-version request-shape permutations in unit tests rather than relying on one Kind image to represent every Kubernetes/OpenShift release.

## Assertions and failure diagnostics

Assert each lifecycle boundary explicitly:
- The initial v1-only state remains a migration no-op with blocking lint, despite target-version 3.6.0.
- After switching to v2 storage, old v1 history blocks lint until migration rewrites data and cleans history.
- Removing v1 before its migration/history cleanup is rejected; removing it after cleanup succeeds.
- After switching to v3 storage, the v1 check passes both before and after the v2-to-v3 migration.
- The migration action removes only stale versions after confirmed rewrites and preserves the actual current storage version.
- Check preserved marker/release fields through the currently served endpoints. The stored encoding is not read directly; a changed resourceVersion shows the object was rewritten.
- Migration/API failure does not falsely clear status.storedVersions.
- Cluster startup, CRD establishment, API discovery, migration wait, and cleanup failures provide enough context in logs to diagnose a CI failure.

## Test implementation guidance

- Use the Go Kind library, not a shell invocation of the kind executable as a substitute for the library.
- Keep setup/teardown isolated and deterministic; use bounded waits and the test context.
- Keep the fixture CRD minimal and compatible across versions. A structural schema with the same simple property in both versions avoids needing conversion code.
- Keep the real upstream SVM controller configuration independent from the mock DSC CRD. It is required only for the SVM API path.
- Reuse shared cluster support if the odh-cli repository already has an appropriate helper; otherwise add a focused test helper with explicit lifecycle ownership.
- Build the CRD in Go and keep other test values in package-level constants, following docs/testing.md.
- Run the commands in-process: build a cobra root as cmd/main.go does, set its streams before adding the lint and migrate commands, and build a new root for every invocation.
- Keep the tests readable as a sequence of named steps (expectLintBlocks, expectStoredVersions, expectCRDUpdateRejected, expectDataPreserved, expectMigratingAgainChangesNothing). Use t.Context() directly; API calls are bounded by the client timeout and the migration by its --timeout flag.
- Record the Kind image/API/controller setup actually used, the exact integration test command, runtime, and cleanup behavior here and in the plan's learning log.

### Choosing the cluster

The suite has two structs: cluster (the connection, the fixtures and the assertions on cluster state) and cli (running the lint and migrate commands in-process against a cluster). Files: storage_migration_test.go, cluster_test.go, cluster_kind_test.go, cluster_external_test.go, cli_test.go.

STORAGE_MIGRATION_CLUSTER selects where the tests run:

- kind (default): every test creates its own Kind cluster and deletes it afterwards.
- external: the tests use the cluster the ambient kubeconfig points at (KUBECONFIG, then ~/.kube/config), the same lookup the CLI does.

On an external cluster:

- The cluster is wiped before each test and again after it: the migration request, the restricted user's ClusterRole and binding, and the DataScienceCluster CRD are deleted. Deleting the CRD deletes every DataScienceCluster. This is a user decision of 2026-10-05; there is no guard for a real ODH or RHOAI installation, so do not point the suite at a cluster where that matters.
- The migration API cannot be switched on or off, so a test is skipped when the cluster does not match what it needs: the two migrate tests need an API, TestMigrateWithoutMigrationAPI needs none.
- Any of the four supported migration APIs counts, so on OpenShift the suite exercises migration.k8s.io/v1alpha1.
- The restricted-user step uses --as, so the caller needs impersonation rights.
- No Kind logs are collected.

### Makefile and CI

- make test/integration runs go test -tags integration on the suite. It does not depend on make build, because no binary is used. Ordinary make test stays usable without a container engine.
- CI has one required storage-migration job that runs make test/integration and uploads the Kind logs on failure. Missing container-engine/API/controller prerequisites must fail this job; do not silently skip coverage.
- Run package tests, make fmt, make lint/fix, make check, make test, and make test/integration. make check runs lint only and skips this suite unless run with GOFLAGS=-tags=integration.

## Dependencies and references

- Blocking validation: [RHOAIENG-98680](dsc-v1-stored-version-validation.md).
- Migration command and unit tests: [RHOAIENG-98681](dsc-storage-version-migration-command.md).
- Shared design and date/status rules: [plan.md](../plan.md).
- Kind Go helper example: https://github.com/lburgazzoli/opendatahub-module-operator/tree/db-service/modules/opendatahub-db-operator/test/support/cluster/kind
- Kubernetes migration guide: https://kubernetes.io/docs/tasks/manage-kubernetes-objects/storage-version-migration/
- KEP-4192: https://www.kubernetes.dev/resources/keps/4192/
- Local repo test conventions: docs/testing.md and docs/quality.md.

## Definition of done

- Kind lifecycle is managed through the Go Kind library and is cleaned up on both success and failure.
- Integration installs the minimal mock CRD with production identity; no full production DSC spec is copied.
- The Go-built CRD fixture exposes only the versions appropriate to each stage, without copying the production spec.
- The real-controller configuration exercises v1-only, v1 + v2 storage, and v2 + v3 storage through the real lint and migrate commands, run in-process. The unavailable-API configuration verifies v1-only no-op and safe failure at v1/v2.
- v1-only migration is a no-op with blocked lint; v1-to-v2 migration clears v1 history and lint; v2-to-v3 migration clears stale v2 while lint already passes.
- Object rewrites (by resourceVersion), marker/release preservation, idempotent reruns, premature version-removal rejection, and permission-failure safety are verified.
- The API scenario has a functioning real upstream controller; API absence fails without changing user objects or history.
- make test/integration and required CI exercise the real migration and unavailable-API scenarios without silent skips.
- Relevant integration tests and repository quality gates pass.

## Execution and resume checklist

- Implement after RHOAIENG-98680 and RHOAIENG-98681. Check for existing lifecycle helpers/tests before adding the focused Kind helper.
- Mark in progress and record the actual start date on the first code implementation change. Add the CRD fixture and lifecycle ownership before the stage assertions.
- Record the actual Kind image, feature gates/runtime configuration, controller, exact test commands, runtime, and cleanup outcome. Planned commands and images are not completed test evidence.
- If interrupted, record which configurations/stages pass, failing logs, outstanding CI wiring, and the next action here and in the parent checkpoint.
- Mark done only when both scenarios and required quality gates pass; copy generalizable findings to the parent and record its final status/date when all tasks are complete.

## Progress and learning record

Append dated updates below. Record status changes, Kind configuration, setup/teardown details, test commands and results, deviations, and testing learnings. On completion, copy generalizable findings into the Development and testing learnings section of ../plan.md.

| Date | Status | Update |
|---|---|---|
| 2026-10-05 | new | Plan initialized from Jira and session. Implementation has not started; no integration tests have run. |
| 2026-10-05 | new | Amended for Kind-only integrations, both real-controller and absence/fallback configurations, embedded in-memory Go templates, actual v1-only → v1/v2 storage → v2/v3 storage stages, action invocation, CI requirements, and resume steps. Implementation and tests have not started. |
| 2026-10-05 | in progress | Suite implemented with the real upstream controller; v2-to-v3 moved to a fresh cluster after the request stalled on a reused one. |
| 2026-10-05 | in progress | Simplified with the user: commands run in-process (no built binary, no host kubectl), CRD built in Go (no embedded template), etcd read removed in favour of comparing resourceVersion, the watch on the request removed, stageContext removed, scenarios turned into three plain test functions, CI matrix reduced to one job, files merged into storage_migration_test.go, cluster_test.go and cluster_cli_test.go. |
| 2026-10-05 | in progress | testCluster split into cluster and cli structs; Kind lifecycle moved to cluster_kind_test.go. Added external-cluster mode (STORAGE_MIGRATION_CLUSTER=external) with a wipe before and after each test. |
| 2026-10-05 | in progress | External mode against a standalone Kind cluster ran v1-to-v2 and then v2-to-v3 on the same cluster without stalling, twice in a row. The difference from the stalled case is that the CRD is deleted and recreated between the two tests. So the stall is tied to carrying one CRD through v1-to-v2 and on to v3, not to a second migration as such. |
| 2026-10-05 | in progress | Tried once more to run v1-to-v2 and v2-to-v3 on a single cluster: the second request stayed Running with a 90s and then a 4-minute timeout. Kept the fresh cluster for v2-to-v3. Cause unknown. |

### Implementation notes

2026-10-05: Added the build-tagged suite and embedded minimal CRD template under tests/integration/storage-migration, the make test/integration target, and CI matrix for api/unavailable. Kind v0.31.0 uses the pinned Kubernetes 1.35.0 node image. Enable StorageVersionMigrator, InformerResourceVersion, and InOrderInformers for the upstream controller; explicitly enable/disable storagemigration.k8s.io/v1beta1 through runtimeConfig. Both configurations booted during the initial run; v1-only tests reached correct storage/lint/no-op behavior, then failed on a nonexistent JSON step-message assertion. That assertion was removed. The next run passed v1-only and v1-to-v2 with the real controller and passed unavailable-API safe failure. v2-to-v3 stalled in Running when reusing the first cluster. The test now starts a fresh Kind cluster with v2-stored data for that separate upgrade path; make test/integration STORAGE_MIGRATION_SCENARIO=api then passed all three stages in 92.59 seconds; both Kind clusters were deleted. The CLI timeout is 90 seconds and request status is logged on failure. The unavailable-API scenario passed during the earlier full run. The suite was later simplified; see the progress table above and the sections of this file for its current shape.

### Tests run

Earlier runs, before the simplification:

- `make test/integration`: v1-only and v1-to-v2 passed with the real controller; unavailable-API failure safety passed; v2-to-v3 stayed Running when the same cluster was reused (167.32s, clusters cleaned up).
- `make test/integration STORAGE_MIGRATION_SCENARIO=api`: all three upgrade states passed with v2-to-v3 in a fresh cluster (92.59s, clusters cleaned up). That environment variable no longer exists.

2026-10-05, Docker 29.7.2, current suite:

- `make test/integration`: passed in 122.8s — TestMigrateV1ToV2 (46.79s), TestMigrateV2ToV3 (45.91s), TestMigrateWithoutMigrationAPI (30.05s). All clusters were deleted.
- `make lint/fix GOFLAGS=-tags=integration`: 0 issues, after wrapping one returned error.

2026-10-05, external mode, after the cluster/cli split:

- `make test/integration` (Kind): passed in 125.8s, all three tests.
- `KUBECONFIG=<file> STORAGE_MIGRATION_CLUSTER=external make test/integration` against a standalone Kind cluster with the migration API: TestMigrateV1ToV2 and TestMigrateV2ToV3 passed, TestMigrateWithoutMigrationAPI skipped (25.4s). A second run on the same cluster gave the same result (39.4s).
- The same against a standalone Kind cluster without the migration API: the two migrate tests skipped, TestMigrateWithoutMigrationAPI passed (3.1s).
- An unknown STORAGE_MIGRATION_CLUSTER value fails with a message naming the valid ones.
- `make lint/fix GOFLAGS=-tags=integration`: 0 issues.

Not run: the CI job, the suite under Podman, and external mode against OpenShift or any cluster with ODH/RHOAI installed.

### Completion notes

_Record implementation summary, review/PR link, final date, and any remaining limitation._

2026-10-05 user correction: removed direct DSC rewrite fallback. The api configuration must complete all three actual upgrade stages; unavailable verifies safe failure only. Unit tests remain ordinary fake-client tests.

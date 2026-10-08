# Task — DSC storage-version migration action and upgrade-flow unit tests

| Field | Value |
|---|---|
| Jira | [RHOAIENG-98681](https://issues.redhat.com/browse/RHOAIENG-98681) |
| Parent | [RHOAIENG-98674](https://issues.redhat.com/browse/RHOAIENG-98674), under [RHOAIENG-94804](https://issues.redhat.com/browse/RHOAIENG-94804) |
| Status | in progress |
| Start date | 2026-10-05 |
| End date | TBD — set when acceptance criteria pass |
| Last updated | 2026-10-05 |
| Assignee / team | Luca Burgazzoli / AI Core Platform |
| Release | 3.6 GA RHOAI RELEASE |

## Objective

Register dsc.storage-version.migrate in the existing odh migrate list/run framework and implement unit tests for the full migration/upgrade safety flow. The action requires a supported migration API and controller selected through live discovery. The CLI never updates DSC custom resources directly.

## Complete context and CLI surface

The target is the DataScienceCluster CRD datascienceclusters.datasciencecluster.opendatahub.io. For the 3.6 upgrade, v3 is storage and v2 remains served; v1 may remain in status.storedVersions even when a v1 resource read would be converted and appear to work.

The user explicitly chose the existing action interface, superseding the initial direct-command proposal derived from Jira wording. The agreed invocation is:

    odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0

Current repository finding: cmd/migrate/migrate.go registers list, prepare, and run. Register the action explicitly in pkg/migrate/registry.go; no separate direct subcommand is required. Use a normal pre-upgrade action with no prepare task and follow the neighboring action/task implementation style. Its CanApply uses the agreed target-at-least-3.6 applicability for version-based listing/selection.

Keep target-version required by the existing runner, which uses it for applicability/phase behavior. The DSC task itself does not use the target release to choose a destination, API request, or rewrite behavior. Read the installed CRD's actual storage version instead, including v2 during an intermediate upgrade. Do not force v3, add an API version, or change spec.versions. On a v1-only CRD with storedVersions [v1], migration is a successful no-op even though the v1 retirement lint check remains blocked.

Reuse the existing task interfaces, step recorder, confirmation/yes behavior, write-free dry-run, output handling, and default 10-minute timeout. Keep new helpers focused on DSC and resource identities centralized; do not refactor the generic runner or shared clients broadly.

Existing stored-version precedent is in pkg/migrate/actions/aipipelines/dspa.go and dspa_test.go. Reuse resource/client/test patterns. DSC reads must surface permission errors, and status pruning uses a fresh CRD read and resourceVersion.

## Runtime API discovery

Discover the APIs served by the connected cluster. Candidate APIs named in the Jira/design are:

- Kubernetes: storagemigration.k8s.io, including the served v1, v1beta1, or v1alpha1 variants.
- OpenShift: migration.k8s.io/v1alpha1.

Do not choose by a hardcoded Kubernetes/OpenShift release matrix or assume feature-gate defaults. Some version/gate and OpenShift details in the supplied design were explicitly unverified. Query discovery, identify the actual served resource, and construct its request using that API version's correct schema. Older request shapes may require group/version/resource; newer shapes may only accept group/resource.

Distinguish these outcomes:
- Discovery succeeded and no supported migration resource is served: fail with instructions to enable the cluster storage migration API and controller. Leave objects and history unchanged.
- Discovery fails, authorization is denied, or the apiserver cannot be reached: return an actionable error.
- API is served but no controller reports success: fail on Failed=True, timeout, or API error. API discovery alone is not evidence that migration completed.

Keep API selection in testable code and use the server discovery/schema rather than a static release-number table.

As built: the action asks discovery about each candidate group/version in turn and uses the first that lists the storageversionmigrations resource. NotFound means that candidate is not served; any other discovery error fails the action.

Choose deterministically from supported served APIs: Kubernetes v1, then v1beta1, then v1alpha1, then OpenShift migration.k8s.io/v1alpha1. Verify each request schema against its API definition and use small constructors; generic schema reflection is not required.

## Safe migration algorithm

1. Read the DataScienceCluster CRD and its current storage version.
2. If status.storedVersions is exactly a single entry equal to the current storage version, report that no migration is needed and exit successfully.
3. For a discovered migration API, create its request with the fixed name dsc-storage-migration, or resume it if it already exists. Poll for Succeeded=True; fail on Failed=True, timeout/cancellation, API or permission error.
4. After successful rewrites, re-read the CRD and prune storedVersions to the current storage version using the fresh resourceVersion. Surface a conflict or unexpected storage-version change as an error; do not restart automatically.
5. Delete the completed request after successful handling. Retain failed or interrupted requests for diagnosis and resumption. Failed requests require review and explicit deletion before retry. Surface cleanup failures.
6. Never prune history after unconfirmed rewrites.

User correction on 2026-10-05: this tool runs on a stable cluster before the upgrade. Use a fixed request name; no UID/generation tracking, automatic restart machinery, or CRD-race test matrix.

## Migration controller requirement

User correction on 2026-10-05 supersedes the earlier fallback design. Remove rewriteObjects and all direct DSC writes; use the migration CR exclusively. A supported API and functioning controller are prerequisites whenever old history needs migration. The controller performs rewrites; the CLI waits for success and updates CRD status using typed UpdateStatus.

## Operational requirements

- Keep the conversion webhook available for the entire rewrite if the CRD uses one. Surface webhook/API failures; never trim status after them.
- Required migration permissions: get customresourcedefinitions; create/get/delete the selected storageversionmigrations resource; update customresourcedefinitions/status. Discovery access is also required. Existing runner DSC reads remain read-only. The command polls requests, so watch is unnecessary.
- Respect context cancellation and report actionable failures for API discovery, RBAC, request creation, Failed=True, timeout, missing APIs/controller, CRD storage changes, status update conflicts, and cleanup.
- Any cleanup or retry behavior for migration resources must make a subsequent command invocation safe.
- Use strict low-level CRD operations through the existing API extensions client; do not silently accept suppressed permission failures. Dry-run and declined confirmation perform no migration-resource, DSC-object, or CRD-status writes.

## Unit and upgrade-flow test matrix

As built, in pkg/migrate/actions/datasciencecluster/storage_version_test.go. Fake clients only: the apiextensions fake clientset, the fake dynamic client and the fake discovery client, wired through client.NewForTesting. One fixture struct holds the target and the typed fakes. A create reactor on the fake dynamic client stands in for the migration controller by storing each request with a condition already set.

| Test | Cases |
|---|---|
| TestAction | ID, phase, no prepare task, CanApply at 3.6 and with no target |
| TestExecuteWithoutWrites | v1 only; already current; dry run; confirmation declined |
| TestExecuteMigrates | each of the four APIs (request GVR, name and shape; history pruned; request deleted); a v2-to-v3 state; preference for Kubernetes v1 when all are served |
| TestExecuteReusesExistingRequest | an existing succeeded request for the same storage version is picked up |
| TestExecuteFailures | no API served; CRD read forbidden; discovery error; existing request for another storage version; create forbidden; controller reports failure; controller never finishes; CRD status update forbidden; request delete forbidden. Each checks the error, the failed step, the history, and whether the request is retained |

Left out on purpose when the tests were simplified: discovery-shape permutations and the target-release matrix (the task never reads the target release). End-to-end upgrade flow and command conventions are covered by the Kind suite.

Use the repository's vanilla Gomega, t.Run, t.Context(), package-level test fixtures, and fake-client conventions. Record exact package-level test commands and results in this file.

## Dependencies and references

- Validation that points users to this command: [RHOAIENG-98680](dsc-v1-stored-version-validation.md).
- Kind end-to-end coverage: [RHOAIENG-98682](kind-integration-tests.md).
- Authoritative safety rules and shared learning: [plan.md](../plan.md).
- Kubernetes storage-version migration guide: https://kubernetes.io/docs/tasks/manage-kubernetes-objects/storage-version-migration/
- Kubernetes CRD versioning: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
- OpenShift StorageVersionMigration API reference: https://docs.redhat.com/en/documentation/openshift_container_platform/4.14/html/storage_apis/storageversionmigration-migration-k8s-io-v1alpha1
- KEP-4192: https://www.kubernetes.dev/resources/keps/4192/
- Cluster API CRD migrator example: https://github.com/kubernetes-sigs/cluster-api/blob/main/controllers/crdmigrator/crd_migrator.go
- Existing odh-cli precedent: pkg/migrate/actions/aipipelines/dspa.go and pkg/migrate/actions/aipipelines/dspa_test.go.
- Current command/action wiring: cmd/migrate/migrate.go, cmd/migrate/run/run.go, pkg/migrate/registry.go, docs/migrate/rhbok-migration.md.

## Definition of done

- dsc.storage-version.migrate is registered and available through the existing migrate list/run framework, with the existing target-version runner requirement retained.
- The task follows the actual CRD storage version and its rewrite logic is independent of target release. A v1-only state is a no-op, not an invented v3 migration.
- Discovery selects only a served, schema-compatible migration API; absence and discovery failure are distinguished.
- No direct DSC updates: use migration CR/controller exclusively and fail safely if unavailable.
- Failed or incomplete operations never prune old status versions.
- The stable-cluster workflow re-reads status before pruning and surfaces status write failures.
- Unit tests use fake clients directly with small fixture helpers and focused tests; no testCluster wrapper or string-driven failure matrix.
- Relevant tests and repository quality gates pass.

## Execution and resume checklist

- Implement after RHOAIENG-98680. Inspect neighboring actions, action.Target/Task, recorder behavior, registry, and AIPipelines precedent before editing.
- Mark in progress and record the actual start date on the first code implementation change. Preserve the runner's existing interfaces.
- Run focused action/command tests, make fmt, make lint/fix, make check, and make test; make check currently runs lint only. Record exact commands and results below.
- If interrupted, record completed helpers, unimplemented API cases, unresolved failures, and the next concrete action here and in the parent checkpoint.
- Once acceptance criteria pass, mark done with evidence and proceed to the Kind task. Do not add envtest or a new generic migration framework.

## Progress and learning record

Append dated updates below. Record status changes, code locations, API discoveries, test commands and results, deviations, and implementation/test learnings. On completion, copy generalizable findings into the Development and testing learnings section of ../plan.md.

| Date | Status | Update |
|---|---|---|
| 2026-10-05 | new | Plan initialized from Jira and session. Implementation has not started; no tests have run. |
| 2026-10-05 | new | Replaced the direct-command proposal with the existing migration action interface. Retained target-version only for runner behavior; documented CRD-derived destinations, request lifecycle, strict pagination/permissions, and resume steps. No implementation or tests have started. |
| 2026-10-05 | in progress | Action implemented and registered; direct object rewrites removed following user feedback. |
| 2026-10-05 | in progress | Readability pass with the user. The steps are methods on a storageVersionMigration struct: run, servedAPI, newRequest, waitForRequest, pruneStoredVersions. Files are storage_version.go (action), storage_version_task.go (task and readStorage) and storage_version_migration.go. Guard clauses and if/else chains became switches; the one-line storageCurrent wrapper was inlined. Discovery asks per candidate group/version. Unit tests consolidated into one file. All gates pass. |

### Implementation notes

2026-10-05: Implementation started in pkg/migrate/actions/datasciencecluster. Registered the pre-upgrade action; added strict CRD reads, discovery, schema-specific requests, condition polling, and typed CRD status updates. The fixed request name is dsc-storage-migration. Direct object rewrites were removed following user feedback. Focused tests use fake clients directly; no testCluster wrapper, failAt helper, CRD UID/generation tracking, or race test matrix. The test fixtures now construct typed CRDs and metav1.Condition values, then place migration requests in the fake client tracker. Focused go test and make check GOFLAGS=-tags=integration passed. The layout described here was later reorganised; see the progress table above for the current files and names.

Verified request schemas: Kubernetes v1 (upstream kubernetes/api master storagemigration/v1/types.go, introduced 1.37) and the pinned v0.35.3 v1beta1 use metav1.GroupResource. Kubernetes v0.34.0 v1alpha1 and OpenShift migration.k8s.io/v1alpha1 use group/version/resource. Selection remains live discovery, independent of cluster or target release.

### Tests run

2026-10-05, after the final code and test changes:

- go test -count=1 -v ./pkg/migrate/actions/datasciencecluster — passed (5 test functions, 17 subtests).
- make fmt — passed.
- make lint/fix — 0 issues.
- make check — 0 issues.
- make test — no failures in the full repository suite.
- make test/integration — passed; see the Kind task.

Only the Kubernetes v1beta1 path has run against a real controller (Kind). The other three APIs, including the OpenShift one, are covered by fake-client tests only.

### Completion notes

_Record implementation summary, review/PR link, final date, and any remaining limitation._

# Task — DSC v1 stored-version validation and unit tests

| Field | Value |
|---|---|
| Jira | [RHOAIENG-98680](https://issues.redhat.com/browse/RHOAIENG-98680) |
| Parent | [RHOAIENG-98674](https://issues.redhat.com/browse/RHOAIENG-98674), under [RHOAIENG-94804](https://issues.redhat.com/browse/RHOAIENG-94804) |
| Status | done |
| Start date | 2026-10-05 |
| End date | 2026-10-05 |
| Last updated | 2026-10-05 |
| Assignee / team | Luca Burgazzoli / AI Core Platform |
| Release | 3.6 GA RHOAI RELEASE |

## Objective

Add the blocking DataScienceCluster v1 stored-version validation to odh-cli's existing odh lint --target-version upgrade-readiness flow. Implement focused unit tests for the check.

This task implements validation only. It does not rewrite custom resources or patch CRD status; that belongs to RHOAIENG-98681.

## Complete context

The RHOAI 3.6 upgrade introduces DataScienceCluster v3 as the storage version while continuing to serve v2 through conversion. Before v1 is dropped, the DataScienceCluster CRD must no longer list v1 in status.storedVersions.

The target CRD is datascienceclusters.datasciencecluster.opendatahub.io (group datasciencecluster.opendatahub.io, plural datascienceclusters, kind DataScienceCluster, cluster-scoped).

The validation question is whether v1 remains in the CRD's status.storedVersions. Do not fetch or list DataScienceCluster objects through v1 to answer it: the apiserver can convert a read to v1 even when the etcd representation is v3. A working v1 read therefore does not prove that no v1 data remains stored.

## Implementation requirements

1. Add platform.dsc.storage-version alongside the existing DSC platform checks in pkg/lint/checks/platform/datasciencecluster, using BaseCheck, existing condition constructors, and version annotations.
2. CanApply uses existing version helpers to apply from any source version to a target at least 3.6, including later major releases. Missing targets and targets below 3.6 do not apply. Do not require the source to be below 3.6 or restrict the target to 3.x; preserve the lint command's existing execution flow.
3. Read the CRD status directly and look for the exact v1 entry in status.storedVersions. Do not inspect DSC object apiVersion or attempt a v1 custom-resource read/list.
4. If v1 is present, produce a failed diagnostic with explicit blocking impact. The existing default for failed conditions is advisory, so explicitly set the blocking impact.
5. Include actionable remediation directing the user to run odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0 before dropping v1. The action's destination is the current CRD storage version, independent of that runner flag.
6. If v1 is absent, produce a passing result.
7. CRD not found, malformed/missing status, authorization failure, or API error returns an execution error. The shared reader can suppress permission failures as a nil object: explicitly reject a nil CRD rather than treating it as a pass.
8. Register the check explicitly in pkg/lint/command.go. Keep lint read-only; no migration, status patch, or custom-resource update belongs in this check.
9. Use centralized GVK/GVR definitions and existing client/discovery helpers when available. Inspect pkg/resources and the existing CRD helpers before adding constants or duplicate utility functions.
10. Read unstructured CRD fields with pkg/util/jq. Do not use DSC-fetching builders or add a generic validation framework for this check.

## Unit test matrix

Add tests for at least these cases:

| Case | Input / setup | Expected result |
|---|---|---|
| Legacy storage remains | CRD status.storedVersions contains v1 alone, v1/v2, or v1/v3 | Failed condition with explicit blocking impact and migration action guidance |
| No legacy storage | CRD status.storedVersions contains only v2 or only v3 | Passing condition |
| v1 is absent among multiple entries | Stored list has v2/v3 but no v1 | Passing for the v1 retirement check |
| CRD read error | Reader returns API/server error | Execution error surfaced; never clean pass |
| Forbidden read | Reader returns authorization failure or a nil CRD from suppressed permission failure | Execution error surfaced; never clean pass |
| Missing CRD or status | CRD absent or status is malformed/missing | Execution error surfaced; never silently treat uncertainty as v1-free |
| No v1 resource read | Fake/spy reader records requested object kinds/versions | Assert the check reads CRD status and does not get/list a DataScienceCluster via v1 |
| Target applicability | Targets below 3.6, 3.6, later 3.x, and later majors; different or missing source versions; missing target | Applicable exactly when target is at least 3.6, without a source gate |

Use the lint test conventions in docs/lint/writing-checks.md and docs/testing.md: vanilla Gomega, t.Run, t.Context(), package-level fixture constants, and fake/read-only clients. Validate DiagnosticResult structure and assert the failed condition's impact is Blocking, not merely Status=False.

## Dependencies and references

- Migration command/remediation details: [RHOAIENG-98681](dsc-storage-version-migration-command.md).
- End-to-end test coverage: [RHOAIENG-98682](kind-integration-tests.md).
- Parent technical contract: [plan.md](../plan.md).
- Kubernetes CRD versioning: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/
- Local implementation guidance: docs/lint/architecture.md, docs/lint/writing-checks.md, docs/testing.md.

## Definition of done

- platform.dsc.storage-version is registered using existing platform check patterns and has the agreed target-at-least-3.6 applicability.
- A stored v1 entry yields a blocking diagnostic with migration guidance.
- No v1 entry yields a pass.
- CRD/API/permission uncertainty does not yield a pass.
- Tests prove the check uses CRD status and does not request a DSC via v1.
- Relevant package tests and repository quality gates pass.

## Execution and resume checklist

- Implement this task first. Check existing code and the working tree before beginning; follow the required project docs and neighboring DSC checks.
- On the first code implementation change, mark this task and the parent in progress and record actual start dates.
- Run focused package tests, make fmt, make lint/fix, make check, and make test; make check currently covers lint only. Record exact commands and outcomes below.
- On interruption, record completed code, pending cases, failures, and the next action here and in the parent resume checkpoint.
- Once acceptance criteria pass, record completion evidence, mark done, and proceed to RHOAIENG-98681. Kind coverage is supplied by RHOAIENG-98682; do not add envtest.

## Progress and learning record

Append dated updates below. Record status changes, code locations, test commands and results, deviations, and implementation/test learnings. On completion, copy generalizable findings into the Development and testing learnings section of ../plan.md.

| Date | Status | Update |
|---|---|---|
| 2026-10-05 | new | Plan initialized from Jira and session. Implementation has not started; no tests have run. |
| 2026-10-05 | new | Amended for platform.dsc.storage-version, any-source/target-at-least-3.6 applicability, registered-action remediation, strict nil/read handling, and sequential execution. Planning-only update; implementation and tests remain unstarted. |
| 2026-10-05 | done | Check implemented, registered, and passing its unit tests and the quality gates. Later the same day the pass/fail branch in Validate was changed from if/else to a switch, with no change in behaviour; tests and lint still pass. The Kind suite exercises the check through the lint command. |

### Implementation notes

2026-10-05: Added the CRD-only check in pkg/lint/checks/platform/datasciencecluster/storage_version.go and explicit registration in pkg/lint/command.go. Focused tests and quality checks are next.

### Tests run

- go test ./pkg/lint/checks/platform/datasciencecluster — passed. Initial sandbox run could not write the Go cache; rerun with approved cache access passed.
- make fmt — passed.
- make lint/fix — found one magic-number violation; replaced target major/minor with named constants.
- make check — passed, 0 issues.
- make test — passed the full repository suite.

### Completion notes

2026-10-05: Registered platform.dsc.storage-version. Unit tests cover v1-containing and v1-free histories, malformed/missing history, nil/denied/error reads, CRD-only access, and applicability across source and target releases. Kind CLI coverage follows in RHOAIENG-98682. Changes remain local; no PR created.

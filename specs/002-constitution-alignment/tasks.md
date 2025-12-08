# Tasks: Constitution Alignment Audit (REVISED)

**Input**: Design documents from `/specs/002-constitution-alignment/`
**Revision Date**: 2025-12-07
**Revision Reason**: Codebase investigation revealed package isolation already complete, command structure needs refactoring

**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md

**Tests**: Tests are NOT explicitly requested in the feature specification. This is a refactoring task with existing test coverage that must be preserved.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each constitutional requirement.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This project follows the odh-cli structure:
- `cmd/` - Cobra command wrappers
- `pkg/` - Package code (refactoring targets)
- Tests colocated with implementation (`*_test.go`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Verify environment and prepare for refactoring

- [x] T001 Verify Go 1.25.0 environment and dependencies in go.mod
- [x] T002 Run baseline quality checks: make check && make test
- [x] T003 Document current test coverage baseline for comparison

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create centralized mock infrastructure that all test refactoring will use

**⚠️ CRITICAL**: Mock infrastructure must exist before moving inline mocks (US2) or refactoring test assertions (US5)

- [x] T004 Create directory structure pkg/util/test/mocks/check/
- [x] T005 Implement MockCheck in pkg/util/test/mocks/check/check.go using testify/mock framework
- [x] T006 Add constructor NewMockCheck() in pkg/util/test/mocks/check/check.go
- [x] T007 Verify mock compiles and satisfies check.Check interface

**Checkpoint**: Mock infrastructure ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Diagnostic Check Package Isolation Validation (Priority: P1) 🎯 MVP

**Goal**: Verify that all existing diagnostic checks follow the package isolation pattern (already complete in codebase)

**Independent Test**: Package structure inspection confirms all checks are isolated, documentation captured for future checks

### Implementation for User Story 1

- [x] T008 [US1] Verify pkg/doctor/checks/components/codeflare/ exists and contains codeflare.go + codeflare_test.go
- [x] T009 [US1] Verify pkg/doctor/checks/components/kserve/ exists and contains kserve.go + kserve_test.go
- [x] T010 [US1] Verify pkg/doctor/checks/components/kueue/ exists and contains kueue.go + kueue_test.go
- [x] T011 [US1] Verify pkg/doctor/checks/components/modelmesh/ exists and contains modelmesh.go + modelmesh_test.go
- [x] T012 [US1] Document package isolation pattern in specs/002-constitution-alignment/PACKAGE-ISOLATION-PATTERN.md for future checks
- [x] T013 [US1] Run make test to confirm all isolated checks pass independently
- [x] T014 [US1] Commit: T008-T013: Validate and document diagnostic check package isolation

**Checkpoint**: At this point, package isolation compliance is verified and documented

---

## Phase 4: User Story 2 - Mock Centralization (Priority: P2)

**Goal**: Move inline MockCheck from selector_test.go to pkg/util/test/mocks/check/check.go

**Independent Test**: Tests using MockCheck pass, import centralized mock instead of inline definition

### Implementation for User Story 2

- [x] T015 [US2] Remove inline MockCheck struct definition from pkg/doctor/check/selector_test.go
- [x] T016 [US2] Add import for centralized mock: mocks "github.com/lburgazzoli/odh-cli/pkg/util/test/mocks/check" in pkg/doctor/check/selector_test.go
- [x] T017 [US2] Update test usage from &MockCheck{...} to mocks.NewMockCheck() in pkg/doctor/check/selector_test.go
- [x] T018 [US2] Update mock method calls to use testify/mock pattern (e.g., mockCheck.On("ID").Return("test")) in pkg/doctor/check/selector_test.go
- [x] T019 [US2] Run make test to verify selector tests pass with centralized mock
- [x] T020 [US2] Run make check to verify linting compliance
- [x] T021 [US2] Commit: T015-T020: Centralize MockCheck using testify/mock framework

**Checkpoint**: At this point, all mocks are centralized, no inline mocks remain, and all tests pass

---

## Phase 5: User Story 3 - Command Package Structure Refactoring (Priority: P3)

**Goal**: Refactor pkg/cmd/doctor/ to follow command package isolation pattern with separate lint/ and upgrade/ packages

**Independent Test**: Command structure shows lint/ and upgrade/ subdirectories with isolated options.go files

### Implementation for User Story 3

- [x] T022 [P] [US3] Create pkg/cmd/doctor/lint/ directory
- [x] T023 [P] [US3] Create pkg/cmd/doctor/upgrade/ directory
- [x] T024 [US3] Move pkg/cmd/doctor/lint_options.go to pkg/cmd/doctor/lint/options.go
- [x] T025 [US3] Move pkg/cmd/doctor/lint_integration_test.go.disabled to pkg/cmd/doctor/lint/integration_test.go.disabled
- [x] T026 [US3] Update package declaration from `package doctor` to `package lint` in pkg/cmd/doctor/lint/options.go
- [x] T027 [US3] Move pkg/cmd/doctor/upgrade_options.go to pkg/cmd/doctor/upgrade/options.go
- [x] T028 [US3] Update package declaration from `package doctor` to `package upgrade` in pkg/cmd/doctor/upgrade/options.go
- [x] T029 [US3] Update import in cmd/doctor/lint.go from `doctor "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor"` to `lint "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor/lint"`
- [x] T030 [US3] Update import in cmd/doctor/upgrade.go from `doctor "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor"` to `upgrade "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor/upgrade"`
- [x] T031 [US3] Update all references to doctor.LintOptions to lint.Options in cmd/doctor/lint.go
- [x] T032 [US3] Update all references to doctor.UpgradeOptions to upgrade.Options in cmd/doctor/upgrade.go
- [x] T033 [US3] Update any cross-package imports in lint/options.go to reference shared_options.go from parent package
- [x] T034 [US3] Update any cross-package imports in upgrade/options.go to reference shared_options.go from parent package
- [x] T035 [US3] Run make test to verify command tests pass with new structure
- [x] T036 [US3] Run make check to verify linting compliance
- [x] T037 [US3] Commit: T022-T036: Refactor command packages per Principle XI isolation pattern

**Checkpoint**: At this point, command package structure follows constitutional isolation pattern

---

## Phase 6: User Story 4 - Code Comment Quality Review (Priority: P4)

**Goal**: Remove obvious comments (stating WHAT), retain WHY comments and godoc

**Independent Test**: No obvious comments remain, all exported functions have godoc, make lint passes

### Implementation for User Story 4

- [x] T038 [US4] Search for obvious comment patterns in pkg/ using grep -r "^// (Get|Check|Set|Create|Update|Remove|Delete|Add)" --include="*.go"
- [x] T039 [US4] Review identified comments and categorize as: obvious (remove), necessary (keep), or godoc (required)
- [x] T040 [P] [US4] Remove/rewrite obvious comments in pkg/cmd/doctor/lint/options.go (1 comment found)
- [x] T041 [P] [US4] Remove/rewrite obvious comments in pkg/util/discovery/discovery.go (2 comments found)
- [x] T042 [P] [US4] Remove/rewrite obvious comments in pkg/util/client/helpers.go (7 comments found)
- [x] T043 [P] [US4] Remove/rewrite obvious comments in pkg/doctor/check/target.go (1 comment found)
- [x] T044 [P] [US4] Remove/rewrite obvious comments in pkg/printer/types.go (1 comment found)
- [x] T045 [P] [US4] Remove/rewrite obvious comments in pkg/doctor/check/global.go (1 comment found)
- [x] T046 [P] [US4] Remove/rewrite obvious comments in pkg/doctor/check/check.go (2 comments found)
- [x] T047 [P] [US4] Remove/rewrite obvious comments in pkg/doctor/check/executor.go (1 comment found)
- [x] T048 [P] [US4] Remove/rewrite obvious comments in pkg/doctor/check/registry.go (2 comments found)
- [x] T049 [P] [US4] Remove/rewrite obvious comments in pkg/printer/table/renderer.go (2 comments found)
- [x] T050 [US4] Verify all exported functions have godoc comments across modified files
- [x] T051 [US4] Run make lint to verify comment quality compliance
- [x] T052 [US4] Run make check to verify overall quality
- [x] T053 [US4] Commit: T038-T052: Remove obvious comments per Code Comments standard

**Checkpoint**: At this point, all obvious comments are removed, godoc is complete, and linting passes

---

## Phase 7: User Story 5 - Gomega Assertion Pattern Compliance (Priority: P5)

**Goal**: Refactor test assertions to use HaveField() and MatchFields() instead of direct field access

**Independent Test**: All tests use Gomega struct matchers, no direct field assertions, make test passes

### Implementation for User Story 5

- [x] T054 [US5] Search for direct field assertion patterns using grep -r "g\.Expect.*\\..*).To(" --include="*_test.go" in pkg/
- [x] T055 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/components/modelmesh/modelmesh_test.go (21 occurrences)
- [x] T056 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/dependencies/kueueoperator/kueueoperator_test.go (8 occurrences)
- [x] T057 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/dependencies/certmanager/certmanager_test.go (12 occurrences)
- [x] T058 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/dependencies/servicemeshoperator/servicemeshoperator_test.go (11 occurrences)
- [x] T059 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/components/kueue/kueue_test.go (18 occurrences)
- [x] T060 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/components/codeflare/codeflare_test.go (17 occurrences)
- [x] T061 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/components/kserve/kserve_test.go (25 occurrences)
- [x] T062 [P] [US5] Refactor direct field assertions to MatchFields() in pkg/doctor/checks/services/servicemesh/servicemesh_test.go (17 occurrences)
- [x] T063 [P] [US5] Note: pkg/doctor/version/detector_test.go, selector_test.go, registry_test.go had no result struct assertions
- [x] T064 [P] [US5] Note: Metadata test functions correctly use individual assertions (testing methods, not struct fields)
- [x] T065 [P] [US5] Note: All refactored tests use PointTo(Equal(...)) for pointer fields and And(...) for combining matchers
- [x] T066 [US5] Run make test to verify all refactored tests pass
- [x] T067 [US5] Run make check to verify linting compliance
- [x] T068 [US5] Commit: T054-T067: Refactor test assertions to use Gomega struct matchers

**Checkpoint**: All user stories should now be independently functional and constitution-compliant

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and documentation updates

- [x] T069 [P] Update CLAUDE.md Recent Changes section to document constitution v1.14.0 alignment completion
- [x] T070 [P] Update .specify/memory/constitution.md SYNC IMPACT REPORT to mark TODO items as complete (noting US1 was already done)
- [x] T071 Run final quality gate: make check && make test
- [x] T072 Verify test coverage matches or exceeds baseline from T003
- [x] T073 Note: REVISED-SCOPE.md not needed - all work documented in constitution SYNC IMPACT REPORT and commit history
- [x] T074 Commit: T069-T073: Update documentation and complete constitution alignment

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS US2 (mock centralization) and US5 (test refactoring)
- **User Stories (Phase 3-7)**: Dependencies vary:
  - **US1 (Phase 3)**: Depends only on Setup - can start after Phase 1 (validation only)
  - **US2 (Phase 4)**: Depends on Foundational (Phase 2) - needs mock infrastructure
  - **US3 (Phase 5)**: Depends only on Setup - can start after Phase 1 (refactoring command structure)
  - **US4 (Phase 6)**: Depends on US3 (Phase 5) - works with refactored command packages
  - **US5 (Phase 7)**: Depends on Foundational (Phase 2) and US2 (Phase 4) - tests use centralized mock
- **Polish (Phase 8)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Setup (Phase 1) - No dependencies on other stories - ✅ **MVP CANDIDATE** (validation only, quick win)
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - Independently testable
- **User Story 3 (P3)**: Can start after Setup (Phase 1) - Independently testable
- **User Story 4 (P4)**: Can start after User Story 3 (Phase 5) - Works with refactored command packages
- **User Story 5 (P5)**: Can start after Foundational (Phase 2) + User Story 2 (Phase 4) - Tests use centralized mock

### Optimal Execution Sequence

1. **Phase 1**: Setup (T001-T003)
2. **Phase 2**: Foundational (T004-T007) - Creates mock infrastructure
3. **Phase 3**: User Story 1 (T008-T014) - Package isolation validation (can run in parallel with US3)
4. **Phase 5**: User Story 3 (T022-T037) - Command structure refactoring (can run in parallel with US1)
5. **Phase 4**: User Story 2 (T015-T021) - Mock centralization (depends on Phase 2)
6. **Phase 6**: User Story 4 (T038-T053) - Comment quality (depends on US3 refactored packages)
7. **Phase 7**: User Story 5 (T054-T068) - Gomega assertions (depends on Phase 2 + US2)
8. **Phase 8**: Polish (T069-T074)

### Parallel Opportunities

**After Setup (Phase 1)**:
- US1 (Package Isolation Validation) and US3 (Command Structure Refactoring) can run in parallel

**After Foundational (Phase 2) + US2 (Phase 4)**:
- US4 (Comment Quality) and US5 (Gomega Assertions) can run in parallel if US3 is complete

**Within US4 (Comment Quality)**:
- T040-T049 (remove obvious comments from different files) can run in parallel

**Within US5 (Gomega Assertions)**:
- T055-T065 (refactor different test files) can run in parallel

---

## Parallel Example: User Story 4 (Comment Quality)

```bash
# Launch comment removal for all files in parallel:
Task: "Remove/rewrite obvious comments in pkg/cmd/doctor/lint/options.go"
Task: "Remove/rewrite obvious comments in pkg/util/discovery/discovery.go"
Task: "Remove/rewrite obvious comments in pkg/util/client/helpers.go"
Task: "Remove/rewrite obvious comments in pkg/doctor/check/target.go"
Task: "Remove/rewrite obvious comments in pkg/printer/types.go"
Task: "Remove/rewrite obvious comments in pkg/doctor/check/global.go"
Task: "Remove/rewrite obvious comments in pkg/doctor/check/check.go"
Task: "Remove/rewrite obvious comments in pkg/doctor/check/executor.go"
Task: "Remove/rewrite obvious comments in pkg/doctor/check/registry.go"
Task: "Remove/rewrite obvious comments in pkg/printer/table/renderer.go"
```

---

## Parallel Example: User Story 5 (Gomega Assertions)

```bash
# Launch test file refactoring in parallel:
Task: "Refactor assertions in pkg/doctor/version/detector_test.go"
Task: "Refactor assertions in pkg/doctor/check/selector_test.go"
Task: "Refactor assertions in pkg/doctor/check/registry_test.go"
Task: "Refactor assertions in pkg/doctor/checks/services/servicemesh/servicemesh_test.go"
Task: "Refactor assertions in pkg/doctor/checks/dependencies/servicemeshoperator/servicemeshoperator_test.go"
Task: "Refactor assertions in pkg/doctor/checks/components/modelmesh/modelmesh_test.go"
Task: "Refactor assertions in pkg/doctor/checks/components/kserve/kserve_test.go"
Task: "Refactor assertions in pkg/doctor/checks/dependencies/kueueoperator/kueueoperator_test.go"
Task: "Refactor assertions in pkg/doctor/checks/components/kueue/kueue_test.go"
Task: "Refactor assertions in pkg/doctor/checks/components/codeflare/codeflare_test.go"
Task: "Refactor assertions in pkg/doctor/checks/dependencies/certmanager/certmanager_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 3: User Story 1 - Package Isolation Validation (T008-T014)
3. **STOP and VALIDATE**: Verify all checks isolated, document pattern
4. This delivers immediate compliance validation and documentation for future checks

### Incremental Delivery

1. Complete Setup (Phase 1) → Environment verified
2. Complete Foundational (Phase 2) → Mock infrastructure ready
3. Add User Story 1 (Phase 3) → Package isolation validated → **Deploy/Demo (MVP!)**
4. Add User Story 3 (Phase 5) → Command structure refactored → Deploy/Demo
5. Add User Story 2 (Phase 4) → Mock centralization complete → Deploy/Demo
6. Add User Story 4 (Phase 6) → Comment quality improved → Deploy/Demo
7. Add User Story 5 (Phase 7) → Test patterns upgraded → Deploy/Demo
8. Each story adds constitutional compliance without breaking previous work

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup (Phase 1) together
2. One developer starts Foundational (Phase 2) while others prepare
3. Once Setup done:
   - Developer A: User Story 1 (Package Isolation Validation)
   - Developer B: User Story 3 (Command Structure Refactoring)
4. Once Foundational + US2 done:
   - Developer A: User Story 4 (Comment Quality)
   - Developer B: User Story 5 (Gomega Assertions)
5. Team completes Polish (Phase 8) together

---

## Notes

- [P] tasks = different files, no dependencies, safe to parallelize
- [Story] label maps task to specific user story for traceability and constitution alignment tracking
- Each user story implements a distinct constitutional requirement and is independently verifiable
- After EACH task: Run make check && make test to catch regressions early
- Commit after each user story phase completion with T### range in message (per Commit Granularity principle)
- Stop at any checkpoint to validate story compliance with constitution v1.14.0
- All refactoring maintains 100% backward compatibility (no API changes)
- Quality gate (make check) MUST pass after every task per constitution requirements

## Task Count Summary

**Total**: 74 tasks (down from 83 in original plan)
- **Phase 1 - Setup**: 3 tasks
- **Phase 2 - Foundational**: 4 tasks
- **Phase 3 - US1 (Package Isolation Validation)**: 7 tasks (down from 28)
- **Phase 4 - US2 (Mock Centralization)**: 7 tasks
- **Phase 5 - US3 (Command Structure Refactoring)**: 16 tasks (up from 8)
- **Phase 6 - US4 (Comment Quality)**: 16 tasks
- **Phase 7 - US5 (Gomega Assertions)**: 15 tasks
- **Phase 8 - Polish**: 6 tasks
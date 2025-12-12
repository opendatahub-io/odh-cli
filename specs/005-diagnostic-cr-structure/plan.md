# Implementation Plan: Diagnostic Result CR Structure

**Branch**: `005-diagnostic-cr-structure` | **Date**: 2025-12-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/005-diagnostic-cr-structure/spec.md`

## Summary

Restructure DiagnosticResult to follow Kubernetes Custom Resource conventions with metadata, spec, and status sections. This change introduces CR-like metadata (Group, Kind, Name, Annotations), a spec section with description, and a status section with a conditions array. Table rendering will display one row per condition for multi-condition checks, enabling granular visibility of validation requirements.

## Technical Context

**Language/Version**: Go 1.23
**Primary Dependencies**:
- k8s.io/apimachinery (metav1.Condition pattern)
- k8s.io/client-go (Kubernetes client)
- github.com/spf13/cobra (CLI framework)
- k8s.io/cli-runtime (kubectl patterns)

**Storage**: N/A (diagnostic results are ephemeral, displayed to stdout)
**Testing**:
- Unit tests: fake client from k8s.io/client-go/dynamic/fake
- Integration tests: k3s-envtest
- Gomega for assertions (vanilla, no Ginkgo)

**Target Platform**: Linux/macOS/Windows servers (kubectl plugin)
**Project Type**: Single CLI project
**Performance Goals**:
- Diagnostic execution: <5 seconds for typical cluster (10-20 checks)
- Table rendering: <1 second for 100 conditions

**Constraints**:
- Zero breaking changes to check interface (checks must adapt to new structure)
- Backward compatible table output format (existing scripts must continue to work)
- Memory: <100MB for large diagnostic result sets (1000+ conditions)

**Scale/Scope**:
- Support 50+ diagnostic checks
- Handle 100+ conditions per diagnostic run
- Scale to clusters with 1000+ workloads

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Phase 0 Gates (Research)

- [x] **Kubectl Plugin Integration**: Diagnostic CR structure aligns with kubectl patterns (CR-like metadata follows Kubernetes conventions)
- [x] **Output Format Consistency**: Table/JSON/YAML output formats maintained (table rendering enhanced for multi-condition display)
- [x] **High-Level Resource Checks**: No low-level Kubernetes primitive checks (diagnostic structure doesn't change check targets)
- [x] **Cluster-Wide Diagnostic Scope**: No namespace filtering (diagnostic structure doesn't affect scope)

**Status**: ✅ PASSED - Feature aligns with constitutional principles

### Phase 1 Gates (Design)

- [x] **Command structure follows Complete/Validate/Run pattern**: No command changes required; check interface remains unchanged
- [x] **Functional options pattern supported**: Not applicable; this is a data structure change, not a command/function initialization pattern
- [x] **Fine-grained package organization**: Changes isolated to pkg/lint/check/ package with focused modules (result.go, metadata.go, condition.go)

**Status**: ✅ PASSED - Design follows constitutional patterns

### Phase 2 Gates (Implementation)

*To be evaluated during implementation*

- [ ] Error handling with fmt.Errorf and %w
- [ ] Test coverage (fake client + k3s-envtest)
- [ ] testify/mock for mocking (mocks in pkg/util/test/mocks)
- [ ] JQ-based field access for unstructured objects
- [ ] Centralized GVK/GVR definitions in pkg/resources/types.go
- [ ] **Kubernetes-native diagnostic CR structure** (Metadata with Group/Kind/Name/Annotations, Spec with Description, Status with Conditions array, table rendering one row per condition)
- [ ] User-facing messages as package-level constants
- [ ] make check execution after each implementation
- [ ] Full linting compliance
- [ ] One commit per completed task with task ID

## Project Structure

### Documentation (this feature)

```text
specs/005-diagnostic-cr-structure/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
pkg/
├── lint/
│   └── check/
│       ├── check.go           # Check interface (updated)
│       ├── result.go          # DiagnosticResult CR structure (NEW)
│       ├── metadata.go        # Metadata struct (NEW)
│       ├── condition.go       # Condition struct (NEW)
│       └── target.go          # CheckTarget struct (existing)
├── cmd/
│   └── lint/
│       └── renderer/
│           └── table.go       # Table renderer (updated for multi-row)
└── util/
    └── test/
        └── mocks/
            └── check/
                └── check.go   # Mock check implementations

tests/
├── unit/
│   └── lint/
│       └── check/
│           ├── result_test.go      # DiagnosticResult tests
│           ├── metadata_test.go    # Metadata tests
│           └── condition_test.go   # Condition tests
└── integration/
    └── lint/
        └── multi_condition_test.go # End-to-end multi-condition tests
```

**Structure Decision**: Single project structure following existing odh-cli organization. Diagnostic CR structure changes are isolated to `pkg/lint/check/` package with minimal impact on command and renderer layers. Test mocks centralized in `pkg/util/test/mocks/check/` following constitution requirements.

## Complexity Tracking

> **No constitutional violations** - Feature aligns with all principles and gates.

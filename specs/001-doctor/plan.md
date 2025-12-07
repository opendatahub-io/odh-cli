# Implementation Plan: Doctor Subcommand

**Branch**: `001-doctor` | **Date**: 2025-12-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/001-doctor/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement a diagnostic system for OpenShift AI clusters with two primary subcommands: `doctor lint` for validating current cluster configuration and `doctor upgrade` for assessing upgrade readiness. The system performs checks across components, services, and workloads using a pluggable check registration mechanism, supports selective check execution, operates fully offline with bundled configurations, and provides actionable remediation guidance with three severity levels (Critical/Warning/Info). The implementation uses client-go dynamic client with unstructured objects, JQ-based field access, and automatic cluster version detection via DataScienceCluster, DSCInitialization, or OLM metadata.

## Technical Context

**Language/Version**: Go 1.24.6
**Primary Dependencies**: client-go v0.34.1 (dynamic client, discovery), k8s.io/apimachinery v0.34.1 (unstructured objects, schema types), github.com/itchyny/gojq v0.12.17 (JQ queries), github.com/spf13/cobra v1.10.1 (CLI framework), k8s.io/cli-runtime v0.34.1 (kubeconfig integration), github.com/blang/semver/v4 (semver constraint matching)
**Storage**: N/A (read-only operations, bundled configuration data embedded or packaged with binary)
**Testing**: Gomega v1.38.2 (assertions without Ginkgo), k8s.io/client-go/dynamic/fake (unit test mocking), NEEDS CLARIFICATION (k3s-envtest for integration tests - constitution mandates lburgazzoli/k3s-envtest)
**Target Platform**: Linux/macOS/Windows (kubectl plugin binary)
**Project Type**: Single (CLI tool following kubectl plugin architecture)
**Performance Goals**: Lint command completes in <2 minutes for typical cluster, upgrade check completes in <3 minutes, selective checks provide 60%+ time reduction
**Constraints**: Read-only Kubernetes permissions (get/list/watch), fully offline operation (no network calls to fetch manifests), must function as kubectl plugin (binary name kubectl-odh, kubeconfig integration)
**Scale/Scope**: Support OpenShift AI 2.x and 3.x versions, handle clusters with 50+ custom resources, extensible check registry for future diagnostic additions

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Phase 0: Research Gate

**kubectl Plugin Integration** (Principle I):
- ✅ Command structure as kubectl plugin: binary name `kubectl-odh`, uses `k8s.io/cli-runtime/pkg/genericclioptions.ConfigFlags` for kubeconfig
- ✅ Leverages existing kubeconfig integration in codebase

**Output Format Consistency** (Principle III):
- ✅ Will support table (default), JSON, YAML output via `-o/--output` flag
- ✅ Aligns with existing printer infrastructure in `pkg/printer/`

**Status**: PASS - Approach aligns with constitution principles

### Phase 1: Design Gate

**Command Structure** (Principle II):
- ✅ Follows Complete/Validate/Run pattern for doctor lint and doctor upgrade commands (see contracts/command-api.md)
- ✅ Command definitions in `cmd/doctor/`, business logic in `pkg/cmd/doctor/` (see plan.md Project Structure)
- ✅ Modular check registry design enables independent testing (see data-model.md CheckRegistry entity)
- ✅ Check interface enables testing without Cobra dependencies (see contracts/check-interface.md)

**Functional Options Pattern** (Principle IV):
- ✅ CheckRegistry uses `Option[T]` pattern for initialization
- ✅ CustomCheck implements functional options for configuration (see contracts/check-interface.md Implementation Patterns)
- ✅ Options defined in `*_options.go` files (lint_options.go, upgrade_options.go, shared_options.go in pkg/cmd/doctor/)

**Status**: PASS - Design conforms to constitution principles

### Phase 2: Implementation Gate

**Error Handling** (Principle V):
- ✅ Will use `fmt.Errorf` with `%w` for error wrapping
- ✅ Context propagation for all client operations (timeout/cancellation support)

**Test Coverage** (Principle VI):
- ✅ Will use vanilla Gomega without Ginkgo
- ✅ Unit tests with `k8s.io/client-go/dynamic/fake` for isolated component testing
- ⚠️ Integration tests MUST use `github.com/lburgazzoli/k3s-envtest` (constitution mandate) - needs dependency addition and test infrastructure setup
- ✅ Test data as package-level constants, subtests via `t.Run()`, context via `t.Context()`

**JQ-Based Field Access** (Principle VII):
- ✅ Will use `pkg/util/jq.Query()` for all unstructured object field access
- ✅ No direct use of `unstructured.NestedField()` or similar accessor methods
- ✅ Existing JQ infrastructure in place

**Centralized GVK/GVR Definitions** (Principle VIII):
- ⚠️ MUST create `pkg/resources/types.go` with all resource type definitions
- ⚠️ All GVK/GVR references MUST use `resources.<ResourceType>.GVK()` and `resources.<ResourceType>.GVR()` accessors
- ⚠️ No direct construction of `schema.GroupVersionResource` or `schema.GroupVersionKind` structs in business logic

**Linting** (Quality Gates):
- ✅ Will comply with `.golangci.yml` configuration (golangci-lint v2)

**Status**: PENDING (will be validated during implementation)

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── main.go                              # Entry point (existing)
├── version/                             # Version command (existing)
└── doctor/                              # NEW: Doctor command definitions
    ├── doctor.go                        # Root doctor command (Cobra wrapper)
    ├── lint.go                          # Lint subcommand (Cobra wrapper)
    └── upgrade.go                       # Upgrade subcommand (Cobra wrapper)

pkg/
├── cmd/
│   └── doctor/                          # NEW: Doctor command business logic
│       ├── lint_options.go              # Lint command options + Complete/Validate/Run
│       ├── upgrade_options.go           # Upgrade command options + Complete/Validate/Run
│       └── shared_options.go            # Shared options (output format, check selection)
├── doctor/                              # NEW: Doctor domain logic
│   ├── check/                           # Check registry and execution
│   │   ├── registry.go                  # Check registration mechanism
│   │   ├── check.go                     # Check interface and base types
│   │   ├── result.go                    # Check result types (pass/fail/error)
│   │   ├── severity.go                  # Severity levels (Critical/Warning/Info)
│   │   └── executor.go                  # Check execution engine
│   ├── checks/                          # Concrete check implementations
│   │   ├── components/                  # Component checks (dashboard, workbenches, etc.)
│   │   ├── services/                    # Service checks (oauth, monitoring, etc.)
│   │   └── workloads/                   # Workload checks (notebooks, inference, etc.)
│   └── version/                         # Cluster version detection
│       ├── detector.go                  # Version detection logic
│       ├── sources.go                   # DataScienceCluster, DSCInitialization, OLM
│       └── version.go                   # Version type and branch mapping
├── resources/                           # NEW: Centralized GVK/GVR definitions
│   └── types.go                         # Resource type definitions (constitution mandate)
├── printer/                             # Existing output formatting
│   ├── table/                           # Table renderer (existing)
│   └── types.go                         # Printer interface (existing)
└── util/                                # Existing utilities
    ├── client/                          # Kubernetes client factory (existing)
    ├── discovery/                       # Discovery utilities (existing)
    ├── jq/                              # JQ query helpers (existing)
    └── option.go                        # Functional options pattern (existing)

internal/
└── version/                             # Version info (existing)
```

**Structure Decision**: Single project (CLI tool) following kubectl plugin conventions. Command layer (`cmd/doctor/`) contains minimal Cobra wrappers. Business logic (`pkg/cmd/doctor/`) implements Complete/Validate/Run pattern. Domain logic (`pkg/doctor/`) provides check registry, execution engine, version detection. All checks implemented in Go code in `pkg/doctor/checks/{category}/`. Centralized resource type definitions in `pkg/resources/types.go` per constitution mandate. All new code integrates with existing infrastructure: `pkg/util/client` for Kubernetes clients, `pkg/util/jq` for unstructured field access, `pkg/printer` for output formatting.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |

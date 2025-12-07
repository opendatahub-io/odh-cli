# Feature Specification: Doctor Subcommand

**Feature Branch**: `001-doctor`
**Created**: 2025-12-06
**Status**: Draft
**Input**: User description: "Implement a doctor subcommand to report potential problems in a running OpenShift AI cluster: - if the user run a lint subcommand (kubectl-odh doctor lint) then the command should perform a linter of the current setup, reporting errors and misconfigurations, eventually providing hints about what to do - if the user run the upgrade subcommand (kubectl-odh doctor upgrade) then the command should provide an overview of the potential misconfigurations or problems to upgrade the current cluster to the selected (via a flag) versions - both the commands should perform a check for components, services and workloads - for workload analysis, the client should use an high level CR, not the low level Pods, Deployments, etc - the git repository for the odh operator is https://github.com/opendatahub-io/opendatahub-operator - the branch for 3.x is main - the branch for 2.x is stable-2.x - an example repo for some simple tests is in https://github.com/lburgazzoli/odh-doctor - it should be possible to execute only a subset of the checks - it should be possible to easily add new checks, so implement a registration mechanism - use client-go, the dynamic client and unstructured Objects - use JQ to read/write values to the unstructured objects instead of the unstructured.Nested/Set helpers - the client should determine the version of the installed cluster by: retrieving from the DataScienceCluster or DSCInitialization status, if not found try to find the version using OLM primitives"

## Clarifications

### Session 2025-12-06

- Q: What output format(s) should the doctor command support for diagnostic results? → A: Both human-readable and JSON (default: human-readable, --output=json flag for automation/scripting)
- Q: What severity levels should be defined for diagnostic check results? → A: Three levels: Critical (blocking issues), Warning (non-blocking problems), Info (optimization suggestions)
- Q: What minimum Kubernetes/OpenShift permissions should the doctor command require to function? → A: Read-only access (get, list, watch on all relevant resources in target namespaces)
- Q: Should the doctor command require network access to fetch operator manifests/configurations, or work offline? → A: Fully offline (bundle expected configurations for known versions, validate against local data only)
- Q: Which specific high-level custom resources should be analyzed for workload checks? → A: All workload-related CRs discovered dynamically at runtime via the `platform.opendatahub.io/part-of` label on deployed CRDs (examples include Notebook, InferenceService, LLMInferenceService, RayCluster, RayJob, RayService, PyTorchJob, TFJob, MPIJob, XGBoostJob, PaddleJob, JAXJob, DataSciencePipelinesApplication, Workflow, Workload, TrustyAIService, GuardrailsOrchestrator, LMEvalJob, ModelRegistry, LlamaStackDistribution, FeatureStore)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cluster Health Validation (Priority: P1)

As a cluster administrator, I want to validate my current OpenShift AI installation to identify configuration errors and misconfigurations so that I can maintain a healthy cluster and prevent operational issues.

**Why this priority**: This is the core diagnostic capability that provides immediate value to administrators managing existing clusters. It addresses the most common operational pain point: detecting problems before they cause failures.

**Independent Test**: Can be fully tested by running the lint command against a test cluster with known misconfigurations and verifying that all errors are detected and reported with actionable guidance.

**Acceptance Scenarios**:

1. **Given** an OpenShift AI cluster with misconfigured components, **When** administrator runs `kubectl-odh doctor lint`, **Then** system detects and reports all configuration errors with severity levels
2. **Given** a correctly configured cluster, **When** administrator runs `kubectl-odh doctor lint`, **Then** system reports clean bill of health with no errors
3. **Given** a cluster with component-specific issues, **When** administrator runs `kubectl-odh doctor lint`, **Then** system provides actionable remediation hints for each detected problem
4. **Given** multiple misconfigurations across different resource types, **When** administrator runs `kubectl-odh doctor lint`, **Then** system reports issues grouped by component/service/workload categories

---

### User Story 2 - Upgrade Readiness Assessment (Priority: P2)

As a cluster administrator planning an upgrade, I want to identify potential compatibility issues and misconfigurations before upgrading to a target version so that I can perform risk-free upgrades.

**Why this priority**: Upgrade planning is critical but less frequent than routine health checks. This builds on P1 functionality by adding version-aware validation.

**Independent Test**: Can be fully tested by running the upgrade command against a test cluster targeting a specific version and verifying that version-specific compatibility issues are identified.

**Acceptance Scenarios**:

1. **Given** a cluster on version 2.x, **When** administrator runs `kubectl-odh doctor upgrade --version 3.0`, **Then** system analyzes configuration against version 3.0 requirements and reports compatibility issues
2. **Given** a cluster ready for upgrade, **When** administrator runs upgrade check, **Then** system confirms cluster is ready with no blocking issues
3. **Given** a cluster with deprecated configurations, **When** administrator checks upgrade readiness, **Then** system identifies deprecated settings that need migration
4. **Given** a target version specified, **When** administrator runs upgrade check, **Then** system validates against version-specific requirements from the appropriate operator branch (main for 3.x, stable-2.x for 2.x)

---

### User Story 3 - Selective Check Execution (Priority: P3)

As a cluster administrator troubleshooting a specific issue, I want to run only relevant diagnostic checks instead of the full suite so that I can quickly identify targeted problems without waiting for comprehensive scans.

**Why this priority**: Improves efficiency for experienced administrators who know which area to focus on. Enhances usability but core value is delivered by P1/P2.

**Independent Test**: Can be fully tested by running commands with check selectors and verifying only specified checks execute and report results.

**Acceptance Scenarios**:

1. **Given** available check categories (components, services, workloads), **When** administrator runs `kubectl-odh doctor lint --checks=components`, **Then** system executes only component-related checks
2. **Given** multiple check selectors, **When** administrator specifies subset of checks, **Then** system runs only selected checks and reports focused results
3. **Given** custom check identifiers, **When** administrator specifies individual check names, **Then** system executes only those specific checks

---

### User Story 4 - Cluster Version Detection (Priority: P1)

As a cluster administrator, I want the doctor command to automatically detect my cluster's OpenShift AI version so that I receive version-appropriate diagnostics without manual configuration.

**Why this priority**: Essential foundational capability that enables version-aware checks. Without accurate version detection, diagnostic accuracy suffers significantly.

**Independent Test**: Can be fully tested by deploying test clusters with different version markers (DataScienceCluster status, DSCInitialization status, OLM metadata) and verifying correct version detection.

**Acceptance Scenarios**:

1. **Given** a cluster with DataScienceCluster resource, **When** doctor command runs, **Then** system extracts version from DataScienceCluster status field
2. **Given** a cluster without DataScienceCluster but with DSCInitialization, **When** doctor command runs, **Then** system extracts version from DSCInitialization status field
3. **Given** a cluster without custom resource version metadata, **When** doctor command runs, **Then** system queries OLM primitives to determine installed operator version
4. **Given** version information from multiple sources, **When** doctor command runs, **Then** system uses the most authoritative source (priority: DataScienceCluster > DSCInitialization > OLM)

---

### Edge Cases

- What happens when cluster version cannot be determined from any source?
- How does system handle checks that fail due to insufficient permissions (when user lacks required read-only access to specific resources)?
- What happens when cluster is in mid-upgrade state with mixed versions?
- What happens when detected cluster version has no bundled configuration data (unsupported or custom version)?
- What happens when custom resources are partially deployed or in error state?
- How does system handle checks when certain components are intentionally disabled?
- What happens when executing checks against clusters with custom/unsupported configurations?
- How does system handle timeout scenarios for slow-responding cluster APIs?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a `doctor lint` subcommand that validates current cluster configuration
- **FR-002**: System MUST provide a `doctor upgrade` subcommand that assesses upgrade readiness to specified target versions
- **FR-003**: System MUST check three categories: components, services, and workloads
- **FR-004**: System MUST analyze workloads using high-level custom resources discovered via the `platform.opendatahub.io/part-of` label on CRDs (examples include Notebook, InferenceService, LLMInferenceService, RayCluster, RayJob, PyTorchJob, TFJob, MPIJob, XGBoostJob, DataSciencePipelinesApplication, Workflow, Workload, TrustyAIService, ModelRegistry, LlamaStackDistribution, FeatureStore) rather than low-level Pods, Deployments, or StatefulSets
- **FR-005**: System MUST determine cluster version by checking DataScienceCluster status, then DSCInitialization status, then OLM metadata in that priority order
- **FR-006**: System MUST support selective check execution via command-line flags or parameters
- **FR-007**: System MUST provide a registration mechanism allowing new checks to be added without modifying core command logic
- **FR-008**: System MUST report detected errors and misconfigurations with three severity levels: Critical (blocking issues), Warning (non-blocking problems), Info (optimization suggestions)
- **FR-009**: System MUST provide actionable remediation hints for detected problems
- **FR-010**: System MUST accept version flag for upgrade subcommand to specify target version
- **FR-011**: System MUST validate against version-specific requirements by referencing appropriate operator repository branches (main for 3.x, stable-2.x for 2.x)
- **FR-012**: System MUST handle cases where cluster version cannot be determined
- **FR-013**: System MUST group diagnostic results by category (components/services/workloads)
- **FR-014**: System MUST support running individual or subset of registered checks
- **FR-015**: System MUST report check execution status (success/failure/skipped) for transparency
- **FR-016**: System MUST support both human-readable (default) and JSON output formats via --output flag
- **FR-017**: System MUST function with read-only Kubernetes permissions (get, list, watch) on all relevant resources in target namespaces
- **FR-018**: System MUST operate fully offline by bundling expected configurations for known OpenShift AI versions and validating against local cluster data only
- **FR-019**: System MUST dynamically discover components and services using discovery client (API groups `components.platform.opendatahub.io` and `services.platform.opendatahub.io`), and workloads using CRD label selector `platform.opendatahub.io/part-of`

### Key Entities

- **Check**: Represents an individual diagnostic test that validates a specific aspect of cluster configuration. Attributes include: unique identifier, category (component/service/workload), severity level (Critical/Warning/Info), description, validation logic, remediation hints
- **Cluster Version**: Represents the detected OpenShift AI version. Attributes include: version number, source of detection (DataScienceCluster/DSCInitialization/OLM), branch mapping (2.x → stable-2.x, 3.x → main)
- **Diagnostic Result**: Represents the outcome of a check execution. Attributes include: check identifier, status (pass/fail/error), detected issues, severity level (Critical/Warning/Info), remediation hints, affected resources
- **Severity Level**: Classification of diagnostic findings - Critical (blocking issues requiring immediate action), Warning (non-blocking problems needing attention), Info (optimization suggestions and best practices)
- **Check Registry**: Collection of available checks organized by category. Enables dynamic check registration and selective execution
- **Workload Custom Resources**: High-level CRs representing AI/ML workloads, discovered dynamically at runtime by querying CustomResourceDefinitions with the `platform.opendatahub.io/part-of` label. Organized by type - Development (Notebook), Model Serving (InferenceService, LLMInferenceService), Distributed Computing (RayCluster, RayJob, RayService), Training (PyTorchJob, TFJob, MPIJob, XGBoostJob, PaddleJob, JAXJob), Pipelines (DataSciencePipelinesApplication, Workflow), Workload Management (Workload via Kueue), AI Governance (TrustyAIService, GuardrailsOrchestrator, LMEvalJob), Model Registry (ModelRegistry), LLM Stack (LlamaStackDistribution), Feature Store (FeatureStore). This dynamic discovery ensures the doctor command automatically supports new workload types added by the ODH operator without code changes

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrators can identify all configuration errors in a cluster within 2 minutes of running the lint command
- **SC-002**: System detects 95% or more of known misconfiguration patterns without false negatives
- **SC-003**: System accurately determines cluster version in 100% of supported installation scenarios (DataScienceCluster, DSCInitialization, or OLM-based deployments)
- **SC-004**: Administrators can assess upgrade readiness for a target version in under 3 minutes
- **SC-005**: System provides actionable remediation guidance for 90% or more of detected issues
- **SC-006**: Selective check execution reduces scan time by at least 60% compared to full suite when running targeted diagnostics
- **SC-007**: New diagnostic checks can be added to the registry and become functional without requiring core code changes
- **SC-008**: 85% of detected issues include specific resource references and clear next steps for resolution

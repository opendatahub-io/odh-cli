# Feature Specification: Diagnostic Result CR Structure

**Feature Branch**: `005-diagnostic-cr-structure`
**Created**: 2025-12-10
**Completed**: 2025-12-12
**Status**: ✅ **Completed**
**Input**: User description: "Restructure DiagnosticResult as Kubernetes CR with metadata (Group, Kind, Name) and status conditions"

## Clarifications

### Session 2025-12-10

- Q: Should Condition.Status represent whether the condition is met (True=passing) or check result polarity (True=failed)? → A: Status represents whether condition is MET (True=passing, False=failing, Unknown=unable to determine) following Kubernetes metav1.Condition semantics
- Q: What happens when a diagnostic check has zero conditions? → A: Zero conditions is invalid; diagnostic checks MUST have at least one condition (validation error if empty)
- Q: How are conditions ordered in the status.conditions array? → A: Conditions ordered by check execution sequence (order checks were run) for diagnostic reproducibility
- Q: Can the same check Name exist across different Groups or Kinds? → A: Same Name can exist across different Group/Kind combinations (unique within Group+Kind tuple)

## User Scenarios & Testing

### User Story 1 - View Diagnostic Results as CR-like Resources (Priority: P1)

As a platform operator, I need diagnostic results structured like Kubernetes Custom Resources with metadata (Group, Kind, Name) so that I can intuitively understand and organize diagnostic information using familiar Kubernetes patterns.

**Why this priority**: Core structural change that enables all other diagnostic viewing and processing scenarios. Without CR-like structure, diagnostics don't align with Kubernetes conventions operators already know.

**Independent Test**: Can be fully tested by running a diagnostic check and verifying the result includes Group, Kind, Name metadata fields with appropriate values, delivering a Kubernetes-native diagnostic experience.

**Acceptance Scenarios**:

1. **Given** a diagnostic check is run on the kserve component, **When** viewing the result, **Then** metadata shows Group="components", Kind="kserve", Name="[check-id]"
2. **Given** a diagnostic check is run on the auth service, **When** viewing the result, **Then** metadata shows Group="services", Kind="auth", Name="[check-id]"
3. **Given** multiple diagnostic results, **When** viewing them, **Then** each is clearly identifiable by its Group/Kind/Name metadata
4. **Given** a diagnostic result, **When** examining its structure, **Then** it follows Kubernetes CR conventions with metadata, spec, and status sections

---

### User Story 2 - Understand Multiple Validation Conditions (Priority: P1)

As a platform operator, I need each diagnostic check to report individual validation conditions in a status.conditions array so that I can identify specific failing requirements rather than a single pass/fail result.

**Why this priority**: Essential for actionable diagnostics. Operators need to know exactly which conditions failed, not just that "something" failed. This is critical for troubleshooting.

**Independent Test**: Can be tested by running a diagnostic check with multiple validation requirements and verifying each condition is reported separately in the status.conditions array.

**Acceptance Scenarios**:

1. **Given** a diagnostic check validates multiple requirements, **When** viewing the result, **Then** status.conditions contains one entry per validation requirement
2. **Given** a diagnostic check has 3 validation conditions and 1 fails, **When** viewing the result, **Then** status.conditions shows 3 conditions with appropriate status for each
3. **Given** a diagnostic check with all conditions passing, **When** viewing the result, **Then** status.conditions shows all conditions with passing status
4. **Given** a diagnostic check with mixed pass/fail conditions, **When** viewing the result, **Then** each condition clearly indicates its status and reason

---

### User Story 3 - View Conditions in Table Format (Priority: P2)

As a platform operator, I need diagnostic checks with multiple conditions to render as multiple table rows (one per condition) so that I can see all validation requirements at a glance without navigating nested data structures.

**Why this priority**: Improves usability for table output format, but depends on condition-based status structure being in place first. Enhances visibility but not blocking for basic functionality.

**Independent Test**: Can be tested by running a diagnostic check with multiple conditions and verifying table output shows one row per condition with appropriate details.

**Acceptance Scenarios**:

1. **Given** a diagnostic check has 3 conditions, **When** rendered as a table, **Then** 3 rows are displayed (one per condition)
2. **Given** a diagnostic check has 1 condition, **When** rendered as a table, **Then** 1 row is displayed
3. **Given** multiple diagnostic checks with varying condition counts, **When** rendered as a table, **Then** total rows equal sum of all conditions across all checks
4. **Given** a table row for a condition, **When** viewing it, **Then** it shows the check's Group/Kind/Name and the specific condition details

---

### User Story 4 - Track Version Information via Annotations (Priority: P2)

As a platform operator, I need diagnostic results to include source and target version information in annotations so that I can understand the version context of each check for upgrade planning and compatibility assessment.

**Why this priority**: Important for version-aware diagnostics and upgrade workflows, but can be added after core CR structure is established. Enhances value but not critical for initial functionality.

**Independent Test**: Can be tested by running a diagnostic check and verifying annotations include source-version and target-version with appropriate domain-qualified keys.

**Acceptance Scenarios**:

1. **Given** a diagnostic check is run, **When** viewing annotations, **Then** "check.opendatahub.io/source-version" annotation is present with current version
2. **Given** a diagnostic check is run with --target-version flag, **When** viewing annotations, **Then** "check.opendatahub.io/target-version" annotation is present with target version
3. **Given** version mismatch is detected, **When** viewing diagnostic, **Then** both source and target version annotations are easily accessible for comparison
4. **Given** multiple diagnostics, **When** filtering by version annotations, **Then** results can be grouped by version context

---

### User Story 5 - Understand Check Purpose via Spec Description (Priority: P3)

As a platform operator, I need each diagnostic result to include a detailed description in the spec section so that I can understand what the check validates without needing to read documentation or source code.

**Why this priority**: Improves self-service troubleshooting and reduces learning curve, but can be added incrementally after core structure and conditions are working. Nice-to-have for complete user experience.

**Independent Test**: Can be tested by viewing diagnostic results and verifying spec.description clearly explains the check's purpose and significance.

**Acceptance Scenarios**:

1. **Given** a diagnostic result, **When** viewing spec.description, **Then** it clearly explains what aspect of the system is being validated
2. **Given** a diagnostic result, **When** viewing spec.description, **Then** it explains why this check matters (impact of failure)
3. **Given** multiple checks for the same Kind, **When** comparing descriptions, **Then** each check's unique purpose is clearly differentiated
4. **Given** a failing diagnostic, **When** viewing spec.description, **Then** operators understand what needs to be fixed

---

### Edge Cases

- Diagnostic checks with zero conditions are invalid and MUST result in a validation error
- Conditions in status.conditions array are ordered by check execution sequence for reproducibility
- Same Name can exist across different Group/Kind combinations (Group+Kind+Name tuple uniquely identifies diagnostic)
- How does the system handle annotation values that exceed expected character limits or contain special characters?
- How are diagnostics displayed when required metadata fields (Group, Kind, Name) are missing or malformed?
- How does table rendering handle checks with very long condition messages or descriptions?
- What happens when annotations contain duplicate keys or invalid domain-qualified formats?

## Requirements

### Functional Requirements

- **FR-001**: DiagnosticResult MUST include metadata section with Group, Kind, and Name fields
- **FR-002**: Metadata Group field MUST categorize the diagnostic target (e.g., "components", "services", "workloads")
- **FR-003**: Metadata Kind field MUST identify the specific target being checked (e.g., "kserve", "auth", "cert-manager")
- **FR-004**: Metadata Name field MUST identify the specific check being performed (e.g., "configuration-valid", "version-compatibility")
- **FR-004a**: DiagnosticResult MUST be uniquely identified by the tuple (Group, Kind, Name) - same Name can exist across different Group/Kind combinations
- **FR-005**: DiagnosticResult MUST support annotations as key-value pairs in the metadata section
- **FR-006**: Annotations MUST use domain-qualified keys following Kubernetes conventions (domain/key format)
- **FR-007**: DiagnosticResult MUST include annotation for source version using key "check.opendatahub.io/source-version"
- **FR-008**: DiagnosticResult MUST include annotation for target version using key "check.opendatahub.io/target-version"
- **FR-009**: DiagnosticResult MUST include spec section containing a description field
- **FR-010**: Spec description field MUST provide detailed explanation of what the check validates
- **FR-011**: DiagnosticResult MUST include status section containing a conditions array
- **FR-012**: Status conditions array MUST contain metav1.Condition-alike structs for each validation requirement
- **FR-012a**: Status conditions array MUST contain at least one condition (empty conditions array is invalid)
- **FR-013**: Each condition MUST report whether its specific validation requirement is met using Status field (True=requirement met/passing, False=requirement not met/failing, Unknown=unable to determine)
- **FR-014**: Condition struct MUST include fields similar to Kubernetes metav1.Condition (Type, Status, Reason, Message, LastTransitionTime) with Status following standard Kubernetes semantics
- **FR-015**: Table rendering MUST display one row per condition when a diagnostic check has multiple conditions
- **FR-016**: Table output MUST include Group, Kind, Name, and condition details in each row
- **FR-017**: DiagnosticResult structure MUST align with Kubernetes Custom Resource conventions (metadata, spec, status sections)
- **FR-018**: System MUST support multiple conditions per diagnostic check without conflicts
- **FR-019**: System MUST order conditions in the status.conditions array by check execution sequence (order checks were run)
- **FR-020**: Annotations MUST be optional but recommended for version-aware diagnostics

### Key Entities

- **DiagnosticResult**: A CR-like structure representing a single diagnostic check result with metadata, spec, and status sections
- **Metadata**: Contains Group (category), Kind (target), Name (check identifier), and Annotations (key-value metadata)
- **Spec**: Contains Description field explaining what the check validates
- **Status**: Contains Conditions array reporting individual validation requirements
- **Condition**: A metav1.Condition-alike struct representing a single validation requirement with Type, Status (True=passing, False=failing, Unknown=unable to determine), Reason, Message, and LastTransitionTime

## Success Criteria

### Measurable Outcomes

- **SC-001**: Platform operators can identify the target and check type of any diagnostic result within 3 seconds using Group/Kind/Name metadata
- **SC-002**: 100% of diagnostic checks include all required CR-like sections (metadata, spec, status)
- **SC-003**: Operators can identify specific failing conditions without additional tooling or documentation
- **SC-004**: Table output clearly displays all conditions with one row per condition
- **SC-005**: Version information is accessible through annotations in 100% of diagnostic results
- **SC-006**: Diagnostic structure aligns with Kubernetes CR conventions that 95% of operators already understand
- **SC-007**: Operators can filter and organize diagnostics by Group, Kind, or Name metadata
- **SC-008**: Multi-condition diagnostics are rendered as multiple table rows automatically
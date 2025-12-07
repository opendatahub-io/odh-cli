# Contract: Command API

**Feature**: 001-doctor | **Date**: 2025-12-06 | **Type**: CLI Interface

## Overview

Defines the command-line interface contract for the doctor subcommand, including flags, arguments, output formats, and exit codes.

---

## Command Structure

### Root Command

```bash
kubectl-odh doctor [subcommand] [flags]
```

**Description:** Diagnostic tool for OpenShift AI clusters

**Global Flags:**
- `--kubeconfig string`: Path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)
- `--context string`: Kubernetes context to use
- `--namespace string, -n string`: Limit workload checks to specified namespace (default: all namespaces). Component and service checks are cluster-scoped and ignore this flag.

---

## Subcommands

### lint

Validates current cluster configuration and reports errors and misconfigurations.

**Usage:**
```bash
kubectl-odh doctor lint [flags]
```

**Flags:**
- `--output, -o string`: Output format - table (default), json, yaml
- `--checks string`: Glob pattern to filter which checks to run (default: "*" - all checks)
  - Supports category shortcuts, exact IDs, and glob patterns
  - Examples: `components`, `components.*`, `*dashboard*`, `components.dashboard`
- `--severity string`: Filter results by minimum severity - critical, warning, info (default: all)
- `--fail-on-critical bool`: Exit with non-zero code if Critical findings detected (default: true)
- `--fail-on-warning bool`: Exit with non-zero code if Warning findings detected (default: false)

**Examples:**
```bash
# Run all checks with default table output
kubectl-odh doctor lint

# Run only component checks (category shortcut)
kubectl-odh doctor lint --checks=components

# Run only component checks (glob pattern)
kubectl-odh doctor lint --checks="components.*"

# Run dashboard-related checks across all categories
kubectl-odh doctor lint --checks="*dashboard*"

# Run specific check by exact ID
kubectl-odh doctor lint --checks=components.dashboard

# Output as JSON for automation
kubectl-odh doctor lint --output=json

# Only show Critical findings
kubectl-odh doctor lint --severity=critical

# Run in specific namespace
kubectl-odh doctor lint --namespace=opendatahub

# Exit with error only on Critical findings
kubectl-odh doctor lint --fail-on-critical=true --fail-on-warning=false
```

**Output (Table Format):**
```
CHECK                              STATUS  SEVERITY  MESSAGE
component-dashboard-deployment-... Pass    -         Dashboard deployment is healthy
component-workbenches-crd-estab... Pass    -         Workbenches CRD is established
service-oauth-client-exists        Fail    Critical  OAuth client odh-dashboard not found
workload-notebook-resource-limits  Fail    Warning   3/5 notebooks missing resource limits

Summary: 2 Pass, 2 Fail (1 Critical, 1 Warning)
```

**Output (JSON Format):**
```json
{
  "version": "3.1.0",
  "versionSource": "DataScienceCluster",
  "timestamp": "2025-12-06T10:30:00Z",
  "results": [
    {
      "checkID": "component-dashboard-deployment-exists",
      "checkName": "Dashboard Deployment Exists",
      "status": "pass",
      "message": "Dashboard deployment is healthy",
      "executionTime": "150ms",
      "timestamp": "2025-12-06T10:30:00Z"
    },
    {
      "checkID": "service-oauth-client-exists",
      "checkName": "OAuth Client Exists",
      "status": "fail",
      "severity": "Critical",
      "message": "OAuth client odh-dashboard not found",
      "affectedResources": [
        {
          "apiVersion": "oauth.openshift.io/v1",
          "kind": "OAuthClient",
          "name": "odh-dashboard"
        }
      ],
      "remediationHint": "Check OAuth client configuration: kubectl get oauthclient odh-dashboard -o yaml",
      "executionTime": "200ms",
      "timestamp": "2025-12-06T10:30:01Z"
    }
  ],
  "summary": {
    "total": 10,
    "pass": 7,
    "fail": 2,
    "error": 0,
    "skipped": 1,
    "critical": 1,
    "warning": 1,
    "info": 0
  }
}
```

**Exit Codes:**
- `0`: All checks passed OR only non-critical findings (respects --fail-on-* flags)
- `1`: Critical findings detected (when --fail-on-critical=true)
- `2`: Warning findings detected (when --fail-on-warning=true)
- `3`: Command execution error (permissions, network, invalid arguments)

---

### upgrade

Assesses upgrade readiness for a target OpenShift AI version.

**Usage:**
```bash
kubectl-odh doctor upgrade --version <target-version> [flags]
```

**Required Flags:**
- `--version string`: Target version for upgrade (e.g., "3.0", "3.1", "2.15")

**Optional Flags:**
- `--output, -o string`: Output format - table (default), json, yaml
- `--checks string`: Glob pattern to filter which checks to run (default: "*" - all checks)
  - Supports category shortcuts, exact IDs, and glob patterns (same as lint command)
- `--severity string`: Filter results by minimum severity - critical, warning, info (default: all)
- `--fail-on-critical bool`: Exit with non-zero code if Critical findings detected (default: true)
- `--fail-on-warning bool`: Exit with non-zero code if Warning findings detected (default: false)

**Examples:**
```bash
# Check upgrade readiness to version 3.0
kubectl-odh doctor upgrade --version=3.0

# Check upgrade to 3.1 with JSON output
kubectl-odh doctor upgrade --version=3.1 --output=json

# Only show blocking issues for upgrade
kubectl-odh doctor upgrade --version=3.0 --severity=critical

# Run specific upgrade-related checks using pattern
kubectl-odh doctor upgrade --version=3.0 --checks="*upgrade*"
```

**Output (Table Format):**
```
UPGRADE READINESS: 2.10.0 → 3.0.0

CHECK                              STATUS  SEVERITY  MESSAGE
upgrade-api-version-deprecated     Fail    Critical  Notebook CR uses deprecated v1alpha1
upgrade-component-renamed          Fail    Warning   ModelMesh component renamed to ModelServing in 3.0
upgrade-storage-migration-required Pass    -         No storage migration needed
upgrade-operator-version-support   Pass    -         Cluster can upgrade to 3.0

Summary: 2 Pass, 2 Fail (1 Critical, 1 Warning)
Recommendation: Address 1 Critical issue before upgrading
```

**Output (JSON Format):**
```json
{
  "currentVersion": "2.10.0",
  "targetVersion": "3.0.0",
  "versionSource": "DataScienceCluster",
  "timestamp": "2025-12-06T10:30:00Z",
  "upgradeRecommendation": "Address 1 Critical issue before upgrading",
  "results": [...],
  "summary": {
    "total": 8,
    "pass": 5,
    "fail": 2,
    "error": 0,
    "skipped": 1,
    "critical": 1,
    "warning": 1,
    "info": 0
  }
}
```

**Exit Codes:**
- `0`: Cluster is ready for upgrade OR only non-blocking issues (respects --fail-on-* flags)
- `1`: Blocking issues detected (when --fail-on-critical=true)
- `2`: Warning-level issues detected (when --fail-on-warning=true)
- `3`: Command execution error (permissions, network, invalid arguments)

---

## Check Selection Patterns

The `--checks` flag accepts a single pattern using glob syntax (powered by Go's `path.Match()`):

**Pattern Types:**

1. **Wildcard (default):**
   - `*`: Matches all checks (default behavior)

2. **Category Shortcuts:**
   - `components`: All component checks (matches `CategoryComponent`)
   - `services`: All service checks (matches `CategoryService`)
   - `workloads`: All workload checks (matches `CategoryWorkload`)
   - `dependencies`: All dependency checks (matches `CategoryDependency`)

3. **Exact ID Match:**
   - `components.dashboard`: Single specific check by ID

4. **Glob Patterns:**
   - `components.*`: All checks in components category (prefix match)
   - `*dashboard*`: All checks containing "dashboard" (substring match)
   - `*.dashboard`: All checks ending with ".dashboard" (suffix match)
   - `components.dash*`: Component checks starting with "dash" (prefix within category)

**Pattern Matching:**
- Patterns match against check IDs (e.g., `components.dashboard`, `services.oauth`)
- Category shortcuts match against check category field
- Glob patterns use standard wildcards: `*` (any characters), `?` (single character), `[...]` (character class)
- Matching is case-sensitive
- Pattern validation happens at command startup (invalid patterns fail fast)

**Examples:**
```bash
# Run all checks (default)
kubectl-odh doctor lint

# Category shortcuts
kubectl-odh doctor lint --checks=components
kubectl-odh doctor lint --checks=services
kubectl-odh doctor lint --checks=workloads
kubectl-odh doctor lint --checks=dependencies

# Glob patterns - prefix match
kubectl-odh doctor lint --checks="components.*"

# Glob patterns - substring match
kubectl-odh doctor lint --checks="*dashboard*"

# Glob patterns - suffix match
kubectl-odh doctor lint --checks="*.oauth"

# Exact ID match
kubectl-odh doctor lint --checks=components.dashboard

# Complex glob
kubectl-odh doctor lint --checks="components.dash*"
```

**Invalid Patterns:**
```bash
# Empty pattern (error)
kubectl-odh doctor lint --checks=""
Error: check selector cannot be empty

# Malformed glob (error)
kubectl-odh doctor lint --checks="["
Error: invalid check selector pattern "[": syntax error in pattern
```

**Performance:**
Selective check execution significantly reduces execution time:
- Full suite: ~2 minutes (all checks)
- Category filter (`--checks=components`): ~40 seconds (60%+ faster)
- Specific check (`--checks=components.dashboard`): <5 seconds (95%+ faster)

---

## Output Format Contracts

### Table Format (Default)

**Requirements:**
- Human-readable columnar layout
- Columns: CHECK (truncated to 35 chars), STATUS, SEVERITY, MESSAGE (truncated to 60 chars)
- Color coding (when terminal supports):
  - Green: Pass status
  - Red: Fail status (Critical)
  - Yellow: Fail status (Warning)
  - Blue: Fail status (Info)
  - Gray: Skipped status
- Summary line at end with counts
- For upgrade command: include current→target version header

**Truncation:**
- CHECK names truncated to 35 characters with "..." suffix
- MESSAGE truncated to 60 characters with "..." suffix
- Full details available in JSON/YAML output

### JSON Format

**Requirements:**
- Valid JSON conforming to schema
- Top-level fields:
  - `version` (string): Detected cluster version
  - `versionSource` (string): How version was detected
  - `timestamp` (string): ISO 8601 timestamp
  - `results` (array): Array of DiagnosticResult objects
  - `summary` (object): Aggregate counts
- For upgrade command: additional fields `currentVersion`, `targetVersion`, `upgradeRecommendation`
- Machine-parsable for automation/scripting
- Pretty-printed for readability (4-space indentation)

**Schema:**
```json
{
  "version": "string",
  "versionSource": "DataScienceCluster|DSCInitialization|OLM",
  "timestamp": "ISO 8601 timestamp",
  "results": [
    {
      "checkID": "string",
      "checkName": "string",
      "status": "pass|fail|error|skipped",
      "severity": "Critical|Warning|Info|null",
      "message": "string",
      "affectedResources": [
        {
          "apiVersion": "string",
          "kind": "string",
          "name": "string",
          "namespace": "string?"
        }
      ],
      "remediationHint": "string",
      "executionTime": "duration string",
      "timestamp": "ISO 8601 timestamp"
    }
  ],
  "summary": {
    "total": "number",
    "pass": "number",
    "fail": "number",
    "error": "number",
    "skipped": "number",
    "critical": "number",
    "warning": "number",
    "info": "number"
  }
}
```

### YAML Format

**Requirements:**
- Valid YAML with same structure as JSON
- Human-readable for inspection
- Machine-parsable for tools like yq

---

## Error Handling

### Invalid Arguments

```bash
$ kubectl-odh doctor upgrade
Error: required flag --version not provided
Usage: kubectl-odh doctor upgrade --version <target-version> [flags]
```

**Exit Code:** 3

### Permission Errors

```bash
$ kubectl-odh doctor lint
Error: insufficient permissions to access cluster resources
Required permissions: get, list, watch on customresourcedefinitions
```

**Exit Code:** 3

### Cluster Unreachable

```bash
$ kubectl-odh doctor lint
Error: unable to connect to cluster: dial tcp 10.0.0.1:6443: i/o timeout
Check your kubeconfig and cluster connectivity
```

**Exit Code:** 3

### Version Detection Failure

```bash
$ kubectl-odh doctor lint
Warning: unable to detect cluster version from any source
Proceeding with generic checks only (some checks may be skipped)
```

**Exit Code:** 0 (continues with limited functionality)

---

## Performance Expectations

**Lint Command:**
- Typical cluster (20-30 checks): < 2 minutes
- Large cluster (50+ checks): < 5 minutes
- Selective checks (`--checks=components`): 60%+ faster than full suite

**Upgrade Command:**
- Typical assessment: < 3 minutes
- Version-specific checks only (less than lint)

**Progress Indication:**
When terminal supports TTY, display progress:
```
Running checks... [15/30] component-workbenches-crd-established
```

---

## Compatibility

**Minimum Kubernetes Version:** 1.28+
**Supported OpenShift AI Versions:** 2.x, 3.x
**Required Permissions:** Read-only (get, list, watch) on all relevant resources

**Kubectl Plugin Discovery:**
Binary must be named `kubectl-odh` and placed in PATH for automatic discovery by `kubectl plugin list` and `kubectl odh doctor` invocation.

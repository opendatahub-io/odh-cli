# Lint Command Architecture

**Scope:** This document describes the architecture of the **lint command** diagnostic system.

The lint command (`kubectl odh lint`) validates OpenShift AI cluster configuration and assesses upgrade readiness. This architecture is specific to the lint command and not a generic diagnostic framework.

For general CLI design, see [../design.md](../design.md). For development practices, see [../development.md](../development.md).

## Check Framework

The lint command diagnostic system is built around a check framework that enables extensible, version-aware validation of OpenShift AI clusters.

### Check Interface

All lint checks implement the `Check` interface:

```go
type Check interface {
    ID() string
    Name() string
    Description() string
    Group() string
    CanApply(ctx context.Context, target *CheckTarget) bool
    Validate(ctx context.Context, target *CheckTarget) *DiagnosticResult
}
```

**Key methods:**
- `ID()` - Unique identifier for the lint check
- `Group()` - Category: "components", "services", or "workloads"
- `CanApply()` - Determines if lint check is applicable based on version context
- `Validate()` - Executes the lint check and returns DiagnosticResult

### CheckTarget

The `CheckTarget` provides lint checks with cluster context:

```go
type CheckTarget struct {
    Client         *client.Client
    CurrentVersion *version.ClusterVersion
    Version        *version.ClusterVersion  // Target version for upgrade checks
}
```

Lint checks compare `CurrentVersion` with `Version` to determine execution mode:
- **Lint mode**: `Version == CurrentVersion` (validate current state)
- **Upgrade mode**: `Version != CurrentVersion` (assess upgrade readiness)

### Check Registration

Lint checks self-register using the init() function pattern:

```go
// pkg/lint/checks/components/kserve/kserve.go
package kserve

import "github.com/lburgazzoli/odh-cli/pkg/lint/check/registry"

func init() {
    registry.Instance().Register(&Check{})
}

type Check struct{}
// ... implementation
```

Command entrypoints use blank imports to trigger registration:

```go
// cmd/lint/lint.go
import (
    // Import check packages to trigger init() auto-registration.
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kserve"
    _ "github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/dashboard"
)
```

**Benefits:**
- Automatic discovery without manual registration
- Extensible: add new lint checks by adding packages
- No modification of core lint command logic required

## DiagnosticResult Structure

DiagnosticResults follow Kubernetes Custom Resource conventions with metadata, spec, and status sections.

### Structure

```go
type DiagnosticResult struct {
    Metadata struct {
        Group       string            // "components", "services", "workloads"
        Kind        string            // Target: "kserve", "dashboard", etc.
        Name        string            // Check identifier
        Annotations map[string]string // Version metadata
    }

    Spec struct {
        Description string // What the lint check validates
    }

    Status struct {
        Conditions []Condition // Individual validation requirements
    }
}

type Condition struct {
    Type               string    // "ConfigurationValid", "ServerlessRemoved", etc.
    Status             string    // "True" (passing), "False" (failing), "Unknown"
    Reason             string    // Machine-readable reason
    Message            string    // Human-readable explanation
    LastTransitionTime time.Time
}
```

### Condition Semantics

Condition `Status` follows Kubernetes metav1.Condition semantics:
- **True**: Requirement is MET (check passing)
- **False**: Requirement is NOT MET (check failing)
- **Unknown**: Unable to determine if requirement is met

### Annotations

Version information is stored in annotations using domain-qualified keys:
- `check.opendatahub.io/source-version` - Current cluster version
- `check.opendatahub.io/target-version` - Target version for upgrade assessment

### Table Rendering

Lint checks with multiple conditions render as multiple table rows (one per condition):

```
GROUP        KIND      NAME                 CONDITION            STATUS   REASON
components   kserve    config-check         ConfigValid          True     ConfigCorrect
components   kserve    config-check         ResourcesAvailable   False    InsufficientMemory
components   kserve    config-check         PermissionsValid     True     PermissionsCorrect
```

This provides at-a-glance visibility of all validation requirements.

## Version Detection

The lint command automatically detects the cluster's OpenShift AI version using a priority-based strategy.

### Detection Priority

1. **DataScienceCluster status** - Primary source
2. **DSCInitialization status** - Fallback if DSC not found
3. **OLM ClusterServiceVersion** - Last resort for operator version

### ClusterVersion Structure

```go
type ClusterVersion struct {
    Version    string         // Semantic version (e.g., "2.17.0")
    Source     VersionSource  // Where version was detected from
    Confidence Confidence     // Detection confidence level
}

type VersionSource string
const (
    SourceDataScienceCluster VersionSource = "DataScienceCluster"
    SourceDSCInitialization  VersionSource = "DSCInitialization"
    SourceOLM                VersionSource = "OLM"
)
```

### Version-to-Branch Mapping

Detected versions map to operator repository branches:
- 2.x versions → `stable-2.x` branch
- 3.x versions → `main` branch

This enables version-specific validation logic in lint checks.

## Resource Discovery

The lint command dynamically discovers components, services, and workloads without hardcoded resource lists.

### Component and Service Discovery

Uses Kubernetes API discovery to find resources:
- **Components**: `components.platform.opendatahub.io` API group
- **Services**: `services.platform.opendatahub.io` API group

```go
// Discover components dynamically
resources, err := client.Discovery().ServerResourcesForGroupVersion("components.platform.opendatahub.io/v1")
```

### Workload Discovery

Discovers workload CRDs via label selector:

```go
// Find all workload CRDs
crdList := &apiextv1.CustomResourceDefinitionList{}
err := client.List(ctx, crdList, &client.ListOptions{
    LabelSelector: labels.SelectorFromSet(labels.Set{
        "platform.opendatahub.io/part-of": "true",
    }),
})
```

Workload types include:
- Development: Notebook
- Model Serving: InferenceService, LLMInferenceService
- Distributed Computing: RayCluster, RayJob, RayService
- Training: PyTorchJob, TFJob, MPIJob, XGBoostJob
- Pipelines: DataSciencePipelinesApplication, Workflow
- AI Governance: TrustyAIService, GuardrailsOrchestrator
- Model Registry: ModelRegistry
- Feature Store: FeatureStore

**Benefits:**
- Automatically supports new components/services added by operator
- No code changes required when new workload types are introduced
- Scales with platform evolution

## Command Lifecycle

The lint command follows a consistent lifecycle pattern with four phases.

### Command Interface

```go
type Command interface {
    Complete() error
    Validate() error
    Run(ctx context.Context) error
    AddFlags(fs *pflag.FlagSet)
}
```

### Lifecycle Phases

1. **AddFlags**: Register lint command-specific flags
   ```go
   func (c *Command) AddFlags(fs *pflag.FlagSet) {
       fs.StringVar(&c.targetVersion, "target-version", "", "Target version")
   }
   ```

2. **Complete**: Initialize runtime state (client, namespace, parsing)
   ```go
   func (c *Command) Complete() error {
       c.client, err = utilclient.NewClient(c.shared.ConfigFlags)
       // Parse flags, populate fields
   }
   ```

3. **Validate**: Verify all required options are set correctly
   ```go
   func (c *Command) Validate() error {
       if !isValidFormat(c.shared.OutputFormat) {
           return fmt.Errorf("invalid output format: %s", c.shared.OutputFormat)
       }
   }
   ```

4. **Run**: Execute lint check logic
   ```go
   func (c *Command) Run(ctx context.Context) error {
       results := c.executeChecks(ctx)
       return c.renderOutput(results)
   }
   ```

### Command Structure

The lint command uses a `Command` struct (not `Options`) with constructor `NewCommand()`:

```go
type Command struct {
    shared        *SharedOptions
    targetVersion string
}

func NewCommand(opts CommandOptions) *Command {
    return &Command{
        shared:        opts.Shared,
        targetVersion: opts.TargetVersion,
    }
}
```

## Output Architecture

The lint command supports three output formats with consistent structure.

### Output Formats

- **Table** (default): Human-readable, one row per condition
- **JSON**: Kubernetes List pattern for scripting
- **YAML**: Kubernetes List pattern for configuration

### JSON/YAML List Structure

Follows Kubernetes List conventions:

```json
{
  "kind": "DiagnosticResultList",
  "metadata": {
    "clusterVersion": "2.17.0",
    "targetVersion": "3.0.0"
  },
  "items": [
    {
      "kind": "DiagnosticResult",
      "metadata": { "group": "...", "kind": "...", "name": "..." },
      "spec": { "description": "..." },
      "status": { "conditions": [...] }
    }
  ]
}
```

**Key characteristics:**
- Results in execution order (sequential, not grouped by category)
- Category information preserved in `metadata.group`
- Deterministic ordering through sequential execution
- Compatible with `jq`/`yq` for post-processing

### Sequential Execution Requirement

**Critical Requirement:** Parallel check execution is PROHIBITED. All lint checks MUST execute sequentially to ensure deterministic ordering.

**Rationale:**
- **Diff-based workflows**: Deterministic output enables meaningful diffs between lint runs
- **Test assertions**: Tests can reliably assert on result order
- **Reproducible diagnostics**: Same cluster state always produces same output order
- **Debugging**: Sequential execution makes it easier to trace check execution flow

**Prohibited:**
```go
// ❌ WRONG: Parallel execution
var wg sync.WaitGroup
for _, check := range checks {
    wg.Add(1)
    go func(c Check) {
        defer wg.Done()
        results <- c.Validate(ctx, target)
    }(check)
}
wg.Wait()
```

**Required:**
```go
// ✓ CORRECT: Sequential execution
for _, check := range checks {
    result := check.Validate(ctx, target)
    results = append(results, result)
}
```

## Offline Operation

The lint command operates **fully offline** by bundling expected configurations for known OpenShift AI versions.

**Requirements:**
- **NO network access** to fetch operator manifests or configurations
- **ALL version configurations** bundled in the binary at compile time
- **Validation against local cluster data only** - no external API calls

**Bundled Configuration:**
```
pkg/lint/config/
├── v2.17/
│   ├── components.yaml    # Expected component configurations
│   ├── services.yaml      # Expected service configurations
│   └── workloads.yaml     # Expected workload types
├── v3.0/
│   ├── components.yaml
│   ├── services.yaml
│   └── workloads.yaml
└── ...
```

**Rationale:**
- **Air-gapped environments**: Works in disconnected clusters
- **Reproducibility**: No dependency on external network state
- **Performance**: No network latency
- **Reliability**: No external service dependencies

## Architectural Principles

### High-Level Resource Targeting

Lint checks operate exclusively on high-level custom resources representing user-facing abstractions.

**Permitted targets:**
- Component CRs (DataScienceCluster, DSCInitialization)
- Workload CRs (Notebook, InferenceService, RayCluster, etc.)
- Service CRs (platform services)
- CRDs, ClusterServiceVersions

**Prohibited targets:**
- Low-level Kubernetes primitives (Pod, Deployment, StatefulSet, Service, ConfigMap, Secret)

**Rationale:** OpenShift AI users interact with high-level CRs, not low-level primitives. Lint checks targeting low-level resources produce noise and don't align with user-facing abstractions.

### Cluster-Wide Scope

The lint command operates cluster-wide and scans all namespaces. Namespace filtering is prohibited.

**Requirements:**
- Component checks examine cluster-scoped resources
- Service checks examine cluster-scoped or all-namespace resources
- Workload checks discover and validate across ALL namespaces
- No `--namespace` or `-n` flags on lint command

**Rationale:** OpenShift AI is a cluster-wide platform. Comprehensive diagnostics require visibility into all namespaces to detect misconfigurations and cross-namespace dependencies.

## Package Organization

### Core Packages

```
pkg/
├── cmd/lint/              # Lint command implementation
├── lint/
│   ├── check/            # Check interface, CheckTarget
│   │   └── registry/     # Check registry
│   ├── version/          # Version detection
│   ├── discovery/        # Resource discovery
│   └── checks/
│       ├── components/   # Component checks (one package per check)
│       ├── services/     # Service checks
│       └── workloads/    # Workload checks
├── printer/              # Output formatting
├── resources/            # Centralized GVK/GVR definitions
└── util/
    ├── jq/              # JQ query utilities
    └── iostreams/       # IOStreams wrapper
```

### Check Package Isolation

Each lint check resides in its own package:

```
pkg/lint/checks/components/
├── dashboard/
│   ├── dashboard.go
│   └── dashboard_test.go
├── kserve/
│   ├── kserve.go
│   └── kserve_test.go
└── modelmesh/
    ├── modelmesh.go
    └── modelmesh_test.go
```

**Benefits:**
- Clear boundaries and dependencies
- Independent testing
- Easy to add/remove lint checks
- Prevents naming conflicts

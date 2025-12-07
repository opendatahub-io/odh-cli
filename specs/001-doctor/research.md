# Research Report: Doctor Subcommand

**Feature**: 001-doctor | **Date**: 2025-12-06 | **Phase**: 0

## Overview

This document consolidates research findings for implementing the doctor subcommand diagnostic system for OpenShift AI clusters. Three key technical areas required investigation: integration testing infrastructure, offline configuration bundling, and dynamic CRD discovery patterns.

---

## Research Area 1: Integration Testing with k3s-envtest

### Decision

Use `github.com/lburgazzoli/k3s-envtest` for integration testing with dynamic clients and unstructured objects.

### Rationale

- **Real Kubernetes environment**: k3s-envtest uses actual k3s containers via testcontainers-go, providing a complete Kubernetes environment with built-in controllers, unlike standard envtest which only runs kube-apiserver and etcd
- **Constitution compliance**: Explicitly mandated by Principle VI of project constitution
- **Minimal dependencies**: Only 10 direct dependencies, seamless client-go dynamic client integration
- **Gomega compatibility**: Works with `NewWithT(t)` pattern for vanilla Go tests without Ginkgo
- **Dynamic client support**: Designed specifically for unstructured objects and dynamic clients

### Alternatives Considered

- **Standard controller-runtime/pkg/envtest**: Rejected because it only provides kube-apiserver/etcd without full controller simulation, and k3s-envtest is constitution-mandated
- **Real cluster testing**: Rejected due to infrastructure complexity, slower execution, and less suitable for CI/CD pipelines

### Implementation Notes

**Dependency Addition:**
```bash
go get github.com/lburgazzoli/k3s-envtest
```

**Test Pattern with Gomega (No Ginkgo):**
```go
func TestDoctorIntegration(t *testing.T) {
    g := gomega.NewWithT(t)
    ctx := t.Context()

    env, err := k3senv.New(
        k3senv.WithManifests("testdata/crds"),
        k3senv.WithLogger(t),
    )
    g.Expect(err).NotTo(gomega.HaveOccurred())

    g.Expect(env.Start(ctx)).To(gomega.Succeed())
    defer func() {
        g.Expect(env.Stop(ctx)).To(gomega.Succeed())
    }()

    dynamicClient, err := dynamic.NewForConfig(env.Config())
    g.Expect(err).NotTo(gomega.HaveOccurred())

    // Run subtests
    t.Run("VersionDetection", func(t *testing.T) {
        testVersionDetection(t, ctx, dynamicClient)
    })
}
```

**Key Patterns:**
- Initialize environment per test or in TestMain
- Use `t.Context()` for context creation (constitution requirement)
- Create dynamic client from `env.Config()`
- Test data as package-level constants (constitution requirement)
- Use `Eventually` for async operations with Gomega argument pattern

**Docker Requirement:** testcontainers-go requires Docker runtime

---

## Research Area 3: Dynamic Resource Discovery

### Decision

Use discovery client for components and services, CRD discovery for workloads:
1. **Components**: Use discovery client to list resources in API group `components.platform.opendatahub.io`
2. **Services**: Use discovery client to list resources in API group `services.platform.opendatahub.io`
3. **Workloads**: Use CRD discovery with label selector `platform.opendatahub.io/part-of` (since workloads come from diverse groups like kubeflow.org, ray.io, etc.)

### Rationale

- **Discovery client for known groups**: Components and services are in specific API groups, so discovery client provides direct resource listing
- **Simple and efficient**: No need to inspect CRDs for components/services, just query the discovery API
- **Label selector for workloads**: Workloads come from diverse groups (kubeflow.org, ray.io, serving.kserve.io, etc.) so CRD label filtering is appropriate
- **Dynamic client compatibility**: Works seamlessly with unstructured objects and existing JQ-based field access (Principle VII)
- **Automatic updates**: New components/services/workloads added to ODH are automatically discovered without code changes

### Alternatives Considered

- **CRD inspection for everything**: Rejected because discovery client is simpler and more direct for known API groups
- **Hardcoded resource list**: Rejected because FR-019 requires dynamic discovery to support new types without code changes

### Implementation Notes

**Discovery Client for Components and Services:**
```go
import (
    "k8s.io/client-go/discovery"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Discover resources by API group using discovery client
func DiscoverResourcesByGroup(client discovery.DiscoveryInterface, apiGroup string) ([]*metav1.APIResource, error) {
    _, apiResourceLists, err := client.ServerGroupsAndResources()
    if err != nil {
        // Partial errors are ok (some groups may be unavailable)
        if !discovery.IsGroupDiscoveryFailedError(err) {
            return nil, fmt.Errorf("failed to discover server resources: %w", err)
        }
    }

    var resources []*metav1.APIResource
    for _, list := range apiResourceLists {
        // Extract group from GroupVersion
        gv, err := schema.ParseGroupVersion(list.GroupVersion)
        if err != nil {
            continue
        }

        if gv.Group == apiGroup {
            for i := range list.APIResources {
                resources = append(resources, &list.APIResources[i])
            }
        }
    }

    return resources, nil
}

// Discover components and services using discovery client
func DiscoverComponentsAndServices(client discovery.DiscoveryInterface) (map[string][]*metav1.APIResource, error) {
    components, err := DiscoverResourcesByGroup(client, "components.platform.opendatahub.io")
    if err != nil {
        return nil, fmt.Errorf("discovering components: %w", err)
    }

    services, err := DiscoverResourcesByGroup(client, "services.platform.opendatahub.io")
    if err != nil {
        return nil, fmt.Errorf("discovering services: %w", err)
    }

    return map[string][]*metav1.APIResource{
        "components": components,
        "services":   services,
    }, nil
}
```

**CRD Discovery for Workloads:**
```go
import (
    apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "github.com/lburgazzoli/odh-cli/pkg/util"
)

// DiscoverGVRsConfig configures CRD discovery
type DiscoverGVRsConfig struct {
    LabelSelector string
}

// Option type using generic pattern from pkg/util
type DiscoverGVRsOption = util.Option[DiscoverGVRsConfig]

// WithCRDLabelSelector filters CRDs by label selector
func WithCRDLabelSelector(selector string) DiscoverGVRsOption {
    return func(c *DiscoverGVRsConfig) {
        c.LabelSelector = selector
    }
}

// DiscoverGVRs discovers custom resources and returns their GVRs
func (c *Client) DiscoverGVRs(ctx context.Context, opts ...DiscoverGVRsOption) ([]schema.GroupVersionResource, error) {
    cfg := &DiscoverGVRsConfig{
        LabelSelector: "platform.opendatahub.io/part-of", // default for workloads
    }
    util.ApplyOptions(cfg, opts...)

    crdList, err := c.APIExtensions.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
        LabelSelector: cfg.LabelSelector,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to list CRDs: %w", err)
    }

    var gvrs []schema.GroupVersionResource
    for i := range crdList.Items {
        crd := &crdList.Items[i]

        // Skip non-established CRDs
        if !isCRDEstablished(crd) {
            continue
        }

        // Extract GVR using storage version
        gvr := crdToGVR(crd)
        gvrs = append(gvrs, gvr)
    }

    return gvrs, nil
}

func crdToGVR(crd *apiextensionsv1.CustomResourceDefinition) schema.GroupVersionResource {
    // Find storage version
    version := ""
    for _, v := range crd.Spec.Versions {
        if v.Storage {
            version = v.Name
            break
        }
    }

    return schema.GroupVersionResource{
        Group:    crd.Spec.Group,
        Version:  version,
        Resource: crd.Spec.Names.Plural,
    }
}

func isCRDEstablished(crd *apiextensionsv1.CustomResourceDefinition) bool {
    for _, condition := range crd.Status.Conditions {
        if condition.Type == apiextensionsv1.Established {
            return condition.Status == apiextensionsv1.ConditionTrue
        }
    }
    return false
}
```

**Usage Example:**
```go
// Discover workload GVRs (uses default label selector)
workloadGVRs, err := client.DiscoverGVRs(ctx)

// Discover with functional option
customGVRs, err := client.DiscoverGVRs(ctx,
    client.WithCRDLabelSelector("custom-label=value"))

// Or pass config struct directly (Option[T] pattern allows both)
customGVRs, err := client.DiscoverGVRs(ctx,
    func(c *client.DiscoverGVRsConfig) {
        c.LabelSelector = "custom-label=value"
    })
```

**Client Structure Extension:**
Add to existing `pkg/util/client/client.go`:
```go
type Client struct {
    Dynamic       dynamic.Interface
    Discovery     discovery.DiscoveryInterface
    APIExtensions apiextensionsclientset.Interface  // NEW
    RESTMapper    meta.RESTMapper                   // NEW for caching
}

// ListResourcesConfig configures resource listing
type ListResourcesConfig struct {
    Namespace     string
    LabelSelector string
    FieldSelector string
}

// Option type using generic pattern from pkg/util
type ListResourcesOption = util.Option[ListResourcesConfig]

// WithNamespace filters resources to a specific namespace
func WithNamespace(ns string) ListResourcesOption {
    return func(c *ListResourcesConfig) {
        c.Namespace = ns
    }
}

// WithLabelSelector filters resources by label selector
func WithLabelSelector(selector string) ListResourcesOption {
    return func(c *ListResourcesConfig) {
        c.LabelSelector = selector
    }
}

// WithFieldSelector filters resources by field selector
func WithFieldSelector(selector string) ListResourcesOption {
    return func(c *ListResourcesConfig) {
        c.FieldSelector = selector
    }
}

// ListResources lists instances of a resource type with optional filters
func (c *Client) ListResources(ctx context.Context, gvr schema.GroupVersionResource, opts ...ListResourcesOption) ([]unstructured.Unstructured, error) {
    cfg := &ListResourcesConfig{}
    util.ApplyOptions(cfg, opts...)

    listOpts := metav1.ListOptions{
        LabelSelector: cfg.LabelSelector,
        FieldSelector: cfg.FieldSelector,
    }

    var list *unstructured.UnstructuredList
    var err error

    if cfg.Namespace != "" {
        list, err = c.Dynamic.Resource(gvr).Namespace(cfg.Namespace).List(ctx, listOpts)
    } else {
        list, err = c.Dynamic.Resource(gvr).List(ctx, listOpts)
    }

    if err != nil {
        return nil, fmt.Errorf("listing resources: %w", err)
    }

    return list.Items, nil
}
```

**Usage Example:**
```go
// List all Notebooks across all namespaces
notebooks, err := client.ListResources(ctx, notebookGVR)

// List Notebooks in specific namespace with functional option
notebooks, err := client.ListResources(ctx, notebookGVR,
    client.WithNamespace("opendatahub"))

// List Notebooks with label filter
notebooks, err := client.ListResources(ctx, notebookGVR,
    client.WithLabelSelector("app=myapp"))

// Combine options
notebooks, err := client.ListResources(ctx, notebookGVR,
    client.WithNamespace("opendatahub"),
    client.WithLabelSelector("app=myapp"))

// Or pass config inline (Option[T] pattern allows both)
notebooks, err := client.ListResources(ctx, notebookGVR,
    func(c *client.ListResourcesConfig) {
        c.Namespace = "opendatahub"
        c.LabelSelector = "app=myapp"
    })
```

**Get Helper Method:**
```go
// GetConfig configures resource retrieval
type GetConfig struct {
    Namespace string
}

// Option type using generic pattern from pkg/util
type GetOption = util.Option[GetConfig]

// InNamespace specifies the namespace for the resource (optional for cluster-scoped)
func InNamespace(ns string) GetOption {
    return func(c *GetConfig) {
        c.Namespace = ns
    }
}

// Get retrieves a single resource by name
func (c *Client) Get(ctx context.Context, gvr schema.GroupVersionResource, name string, opts ...GetOption) (*unstructured.Unstructured, error) {
    cfg := &GetConfig{}
    util.ApplyOptions(cfg, opts...)

    var resource *unstructured.Unstructured
    var err error

    if cfg.Namespace != "" {
        resource, err = c.Dynamic.Resource(gvr).Namespace(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
    } else {
        // Cluster-scoped resource
        resource, err = c.Dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
    }

    if err != nil {
        return nil, fmt.Errorf("getting resource: %w", err)
    }

    return resource, nil
}
```

**Get Usage Examples:**
```go
// Get cluster-scoped resource
dsc, err := client.Get(ctx, dataScienceClusterGVR, "default-dsc")

// Get namespace-scoped resource
deployment, err := client.Get(ctx, deploymentGVR, "odh-dashboard",
    client.InNamespace("opendatahub"))

// Or pass config inline (Option[T] pattern allows both)
deployment, err := client.Get(ctx, deploymentGVR, "odh-dashboard",
    func(c *client.GetConfig) {
        c.Namespace = "opendatahub"
    })
```

**Error Handling Patterns:**
- Check CRD status conditions before listing instances
- Handle `IsNotFound` errors gracefully (no instances is valid)
- Detect `IsForbidden` errors for insufficient permissions (FR-017)
- Report CRDs in error state as Warning/Critical findings

**Caching Strategy:**
Use `restmapper.NewDeferredDiscoveryRESTMapper` with `memory.NewMemCacheClient` for efficient GVK→GVR mapping during command execution.

**Performance Considerations:**
- CRD discovery is one-time operation at command startup
- Cache discovered CRDs for command duration
- Short-lived diagnostic runs minimize caching overhead

---

## Implementation Recommendations

### Integration Testing Setup

1. Add `github.com/lburgazzoli/k3s-envtest` to `go.mod`
2. Create `testdata/crds/` directory for test CRD manifests
3. Implement integration tests in `pkg/doctor/*_integration_test.go` files
4. Use `NewWithT(t)` for Gomega assertions without Ginkgo
5. Ensure Docker is available in CI environment for testcontainers

### Configuration Bundling Setup

1. Create `pkg/doctor/configs/` directory structure with `2.x/`, `3.x/`, `common/` subdirectories
2. Define YAML schema for validation rules (components, services, workloads)
3. Implement `pkg/doctor/config/loader.go` with go:embed and parsing logic
4. Create initial validation rules for known OpenShift AI components
5. Add schema validation tests to ensure YAML integrity

### Workload Discovery Implementation

1. Extend `pkg/util/client/client.go`:
   - Add APIExtensions clientset and RESTMapper fields
   - Add `Client.DiscoverGVRs()` method with `Option[DiscoverGVRsConfig]` pattern
   - Add `Client.ListResources()` method with `Option[ListResourcesConfig]` pattern
   - Both methods use `util.ApplyOptions()` for option processing
2. Implement helper functions: `crdToGVR()`, `isCRDEstablished()`
3. Implement error handling for non-established CRDs
4. Cache discovery results for command execution duration

### Cross-Cutting Concerns

- **Resource Type Centralization (Principle VIII)**: Create `pkg/resources/types.go` for all GVK/GVR definitions
- **JQ Integration (Principle VII)**: Use `pkg/util/jq.Query()` for all unstructured field access in checks
- **Error Wrapping (Principle V)**: Use `fmt.Errorf` with `%w` throughout
- **Context Propagation**: Pass `context.Context` to all client operations

---

## Dependencies to Add

```go
// go.mod additions
require (
    github.com/lburgazzoli/k3s-envtest v0.x.x  // Integration testing
    k8s.io/apiextensions-apiserver v0.34.1     // CRD discovery
    sigs.k8s.io/yaml v1.6.0                    // Config parsing (already in deps)
)
```

---

## Next Phase

With research complete, proceed to **Phase 1: Design & Contracts** to generate:
- `data-model.md`: Entity definitions for Check, CheckRegistry, ClusterVersion, DiagnosticResult, Severity
- `contracts/`: Check interface contracts, configuration schemas
- `quickstart.md`: Getting started guide for users
- Agent context update with Go 1.24.6 technology metadata

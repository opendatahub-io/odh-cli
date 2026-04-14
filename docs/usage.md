# Alternative Usage Methods

For container-based usage (recommended), see the [README](../README.md).

## Using Go Run (No Installation Required)

If you have Go installed, you can run the CLI directly from GitHub without cloning:

```bash
# Show help
go run github.com/opendatahub-io/odh-cli/cmd@latest --help

# Show version
go run github.com/opendatahub-io/odh-cli/cmd@latest version

# Run lint command
go run github.com/opendatahub-io/odh-cli/cmd@latest lint --target-version 3.3.0
```

> **Note:** Replace `@latest` with `@v1.2.3` to run a specific version, or `@main` for the latest development version.

**Token Authentication:**

```bash
go run github.com/opendatahub-io/odh-cli/cmd@latest \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

**Available commands:**
- `lint` - Validate cluster configuration and assess upgrade readiness
- `must-gather` - Collect diagnostic information from OpenShift AI clusters
- `version` - Display CLI version information

## Must-Gather (Collect Diagnostic Information)

Collect diagnostic information from OpenShift AI clusters for troubleshooting:

> **Important:** The must-gather command requires the collection scripts from the [must-gather repository](https://github.com/red-hat-data-services/must-gather).
>
> **Usage options:**
> 1. **Container image (recommended)** - Scripts pre-bundled at `/opt/must-gather`
> 2. **Local binary with mounted scripts** - Use `--scripts-path` flag to point to a local must-gather clone

**Using container (recommended):**

```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  -v ./must-gather.local.$(date +%s):/tmp/must-gather \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev must-gather
```

**Using local binary (requires must-gather scripts):**

```bash
# First, clone the must-gather repository
git clone https://github.com/red-hat-data-services/must-gather.git /tmp/must-gather-scripts

# Run with --scripts-path pointing to the cloned repository
kubectl-odh must-gather --scripts-path /tmp/must-gather-scripts/collection-scripts
```

**For xKS environments (currently OCP, AKS, CKS) - collect LLM-D components only:**

```bash
# Container (recommended)
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  -v ./must-gather.local.$(date +%s):/tmp/must-gather \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev must-gather --component llm-d

# Local binary
kubectl-odh must-gather \
  --scripts-path /tmp/must-gather-scripts/collection-scripts \
  --component llm-d
```

**Collect with time filter:**

```bash
# Container
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  -v ./must-gather.local.$(date +%s):/tmp/must-gather \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev must-gather --since 1h

```

**List available components:**

```bash
kubectl-odh must-gather --list-components # Only llm-d for now
```

> **Note:** `--list-components` doesn't require the must-gather scripts, so it works without `--scripts-path` or the container image.

## As kubectl Plugin

Install the `kubectl-odh` binary to your PATH:

```bash
# Option 1: Download from releases
# Download binary and place in PATH as kubectl-odh

# Option 2: Build locally
make build
sudo cp bin/kubectl-odh /usr/local/bin/

# Use with kubectl
kubectl odh lint --target-version 3.3.0
kubectl odh version
kubectl odh must-gather --list-components
```

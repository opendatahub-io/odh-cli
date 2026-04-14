# odh-cli

CLI tool for RHOAI (Red Hat OpenShift AI) for interacting with RHOAI deployments on Kubernetes.

## Quick Start

### Using Containers

Run the CLI using the pre-built container image:

**Podman:**
```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev lint --target-version 3.3.0
```

**Docker:**
```bash
docker run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev lint --target-version 3.3.0
```

The container has `KUBECONFIG=/kubeconfig` set by default, so you just need to mount your kubeconfig to that path.

> **SELinux:** On systems with SELinux enabled (Fedora, RHEL, CentOS), add `:Z` to the volume mount:
> ```bash
> # Podman
> podman run --rm -ti \
>   -v $KUBECONFIG:/kubeconfig:Z \
>   quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev lint --target-version 3.3.0
>
> # Docker
> docker run --rm -ti \
>   -v $KUBECONFIG:/kubeconfig:Z \
>   quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev lint --target-version 3.3.0
> ```

**Available Tags:**
- `:latest` - Latest stable release
- `:dev` - Latest development build from main branch (updated on every push)
- `:vX.Y.Z` - Specific version (e.g., `:v1.2.3`)

> **Note:** The images are OCI-compliant and work with both Podman and Docker. Examples for both are provided below.

**Shell Access:**

The container also bundles migration tools and CLI utilities that can be used directly from a shell session:

**Podman:**
```bash
podman run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev
```

**Docker:**
```bash
docker run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev
```

Available tools:
- `rhai-cli`
- `kubectl` (latest stable)
- `oc` (latest stable)
- `jq`
- `wget`
- `curl`
- `tar`
- `gzip`
- `bash`

Example usage:
```bash
oc login --token=sha256~xxxx --server=https://api.my-cluster.p3.openshiftapps.com:6443

kubectl get pods -n opendatahub
oc get dsci

rhai-cli lint --target-version 3.3.0
```

The `rhai-cli` binary is located at `/opt/rhai-cli/bin/rhai-cli` (already on `PATH`).
Upgrade helper scripts are located at `/opt/rhai-upgrade-helpers`.
Must-gather scripts are located at `/opt/must-gather`.

**Token Authentication:**

For environments where you have a token and server URL instead of a kubeconfig file:

**Podman:**
```bash
podman run --rm -ti \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

**Docker:**
```bash
docker run --rm -ti \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

## Troubleshooting

### Must-Gather (Collect Diagnostic Information)

Collect diagnostic information from OpenShift AI clusters for troubleshooting.

> **Requirement:** Must-gather needs collection scripts from [must-gather repository](https://github.com/red-hat-data-services/must-gather).
>
> **Note:** Currently only the `llm-d` component is supported.

#### Using Container Image (Recommended)

Scripts are pre-bundled. Output is written to the mounted directory.

**Collect llm-d component:**

**Option 1: Mount output directory (requires permissions)**
```bash
# Create output directory with timestamp and proper permissions
GATHER_DIR="./must-gather.local.$(date +%s)"
mkdir -p "$GATHER_DIR"
chmod 777 "$GATHER_DIR"

podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig:Z -v "$GATHER_DIR":/tmp/must-gather:Z \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev must-gather --component llm-d
```

**Option 2: Copy files after collection (cleaner, no permission issues)**
```bash
# Run collection inside container (without --rm to preserve for copy)
podman run -ti \
  --name odh-must-gather -v $KUBECONFIG:/kubeconfig:Z \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:dev must-gather --component llm-d

# Copy results out thenc lean up container
GATHER_DIR="./must-gather.local.$(date +%s)"
podman cp odh-must-gather:/tmp/must-gather "$GATHER_DIR"
podman rm odh-must-gather
```

> **Note:** The `:Z` flag sets the SELinux label for container access on RHEL/Fedora/CentOS. Omit `:Z` on non-SELinux systems.

#### Using Local Binary (Development)

For development, clone the scripts and use `--scripts-path`:

```bash
# 1. Clone scripts repository
git clone https://github.com/red-hat-data-services/must-gather.git /tmp/must-gather-scripts

# 2. Run with --scripts-path
kubectl-odh must-gather \
  --scripts-path /tmp/must-gather-scripts/collection-scripts \
  --component llm-d
```

## Documentation

For detailed documentation, see:
- [Alternative Usage Methods](docs/usage.md) - Using Go Run, kubectl plugin
- [Design and Architecture](docs/design.md)
- [Development Guide](docs/development.md)
- [Lint Architecture](docs/lint/architecture.md)
- [Writing Lint Checks](docs/lint/writing-checks.md)


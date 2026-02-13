# odh-cli

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes.

## Quick Start

### Using Containers

Run the CLI using the pre-built container image:

**Podman:**
```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest lint --target-version 3.3.0
```

**Docker:**
```bash
docker run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest lint --target-version 3.3.0
```

The container has `KUBECONFIG=/kubeconfig` set by default, so you just need to mount your kubeconfig to that path.

> **SELinux:** On systems with SELinux enabled (Fedora, RHEL, CentOS), add `:Z` to the volume mount:
> ```bash
> # Podman
> podman run --rm -ti \
>   -v $KUBECONFIG:/kubeconfig:Z \
>   quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest lint --target-version 3.3.0
>
> # Docker
> docker run --rm -ti \
>   -v $KUBECONFIG:/kubeconfig:Z \
>   quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest lint --target-version 3.3.0
> ```

**Available Tags:**
- `:latest` - Latest stable release
- `:dev` - Latest development build from main branch (updated on every push)
- `:vX.Y.Z` - Specific version (e.g., `:v1.2.3`)

> **Note:** The images are OCI-compliant and work with both Podman and Docker. Examples for both are provided below.

**Interactive Debugging:**

The container includes kubectl, oc, and debugging utilities for interactive troubleshooting:

**Podman:**
```bash
podman run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest
```

**Docker:**
```bash
docker run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest
```

Once inside the container, use kubectl/oc/wget/curl:
```bash
kubectl get pods -n opendatahub
oc get dsci
kubectl-odh lint --target-version 3.3.0
```

Available tools: `kubectl` (latest stable), `oc` (latest stable), `wget`, `curl`, `tar`, `gzip`, `bash`

**Token Authentication:**

For environments where you have a token and server URL instead of a kubeconfig file:

**Podman:**
```bash
podman run --rm -ti \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

**Docker:**
```bash
docker run --rm -ti \
  quay.io/rhoai/rhoai-upgrade-helpers-rhel9:latest \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

### Using Go Run (No Installation Required)

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
- `version` - Display CLI version information

### As kubectl Plugin

Install the `kubectl-odh` binary to your PATH:

```bash
# Download from releases
# Place in PATH as kubectl-odh
# Use with kubectl
kubectl odh lint --target-version 3.3.0
kubectl odh version
```

## Documentation

For detailed documentation, see:
- [Design and Architecture](docs/design.md)
- [Development Guide](docs/development.md)
- [Lint Architecture](docs/lint/architecture.md)
- [Writing Lint Checks](docs/lint/writing-checks.md)

## Building from Source

```bash
make build
```

The binary will be available at `bin/kubectl-odh`.

## Building Container Image

```bash
make publish
```

This builds a multi-platform container image (linux/amd64, linux/arm64).

# odh-cli

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes.

## Quick Start

### Using Containers

Run the CLI using the pre-built container image:

```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/lburgazzoli/odh-cli:latest lint --target-version 3.3.0
```

The container has `KUBECONFIG=/kubeconfig` set by default, so you just need to mount your kubeconfig to that path.

> **Note:** The images are OCI-compliant and work with both Podman and Docker.

**Interactive Debugging:**

The container includes kubectl, oc, and debugging utilities for interactive troubleshooting:

```bash
# Shell into container
podman run -it --rm \
  -v $KUBECONFIG:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/lburgazzoli/odh-cli:latest

# Inside container, use kubectl/oc/wget/curl
kubectl get pods -n opendatahub
oc get dsci
kubectl-odh lint --target-version 3.3.0
```

Available tools: `kubectl` (latest stable), `oc` (latest stable), `wget`, `curl`, `tar`, `gzip`, `bash`

**Token Authentication:**

For environments where you have a token and server URL instead of a kubeconfig file:

```bash
podman run --rm -ti \
  quay.io/lburgazzoli/odh-cli:latest \
  lint \
  --target-version 3.3.0 \
  --token=sha256~xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --server=https://api.my-cluster.p3.openshiftapps.com:6443
```

### Using Go Run (No Installation Required)

If you have Go installed, you can run the CLI directly from GitHub without cloning:

```bash
# Show help
go run github.com/lburgazzoli/odh-cli/cmd/main.go@latest --help

# Show version
go run github.com/lburgazzoli/odh-cli/cmd/main.go@latest version

# Run lint command
go run github.com/lburgazzoli/odh-cli/cmd/main.go@latest lint --target-version 3.3.0
```

> **Note:** Replace `@latest` with `@v1.2.3` to run a specific version, or `@main` for the latest development version.

**Token Authentication:**

```bash
go run github.com/lburgazzoli/odh-cli/cmd/main.go@latest \
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

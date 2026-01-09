# odh-cli

CLI tool for ODH (Open Data Hub) and RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes.

## Quick Start

### Using Containers

Run the CLI using the pre-built container image:

```bash
podman run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/lburgazzoli/odh-cli:latest lint
```

The container has `KUBECONFIG=/kubeconfig` set by default, so you just need to mount your kubeconfig to that path.

> **Note:** The images are OCI-compliant and work with both Podman and Docker.

**Available commands:**
- `lint` - Validate cluster configuration and assess upgrade readiness
- `version` - Display CLI version information

### As kubectl Plugin

Install the `kubectl-odh` binary to your PATH:

```bash
# Download from releases
# Place in PATH as kubectl-odh
# Use with kubectl
kubectl odh lint
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

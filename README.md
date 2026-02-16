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

**Interactive Debugging:**

The container includes kubectl, oc, and debugging utilities for interactive troubleshooting:

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

## Documentation

For detailed documentation, see:
- [Alternative Usage Methods](docs/usage.md) - Using Go Run, kubectl plugin
- [Design and Architecture](docs/design.md)
- [Development Guide](docs/development.md)
- [Lint Architecture](docs/lint/architecture.md)
- [Writing Lint Checks](docs/lint/writing-checks.md)


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


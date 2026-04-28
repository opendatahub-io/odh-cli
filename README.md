# odh-cli

CLI tool for ODH/RHOAI (Red Hat OpenShift AI) for interacting with ODH/RHOAI deployments on Kubernetes.

## Available Subcommands

- **`odh-cli lint`** - Validate cluster configuration and assess upgrade readiness
- **`odh-cli version`** - Display CLI version information

## Quick Start

### Using Pre-built Binary

Download the latest release from [GitHub Releases](https://github.com/opendatahub-io/odh-cli/releases):

```bash
# Download and extract (example for Linux amd64)
curl -LO https://github.com/opendatahub-io/odh-cli/releases/latest/download/odh-cli_linux_amd64.tar.gz
tar -xzf odh-cli_linux_amd64.tar.gz

# Make it avaiable on PATH
sudo mv odh-cli /usr/local/bin/

# Verify installation
odh-cli version

# Run commands
odh-cli lint --target-version 3.3.0
```

**As kubectl Plugin (optional):**

To use as `kubectl odh`, rename or symlink the binary after download it:

```bash
# Option 1: Rename
sudo mv odh-cli /usr/local/bin/kubectl-odh

# Option 2: Symlink (keeps both names available)
sudo ln -s /usr/local/bin/odh-cli /usr/local/bin/kubectl-odh

# Now use with kubectl
kubectl odh lint --target-version 3.3.0
```

### Using Containers

Run the CLI using the pre-built container image. Set your container runtime (podman or docker):

```bash
# Use podman or docker
CONTAINER_TOOL=podman  # or 'docker'

# Run lint command
$CONTAINER_TOOL run --rm -ti \
  -v $KUBECONFIG:/kubeconfig \
  quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
```

The container has `KUBECONFIG=/kubeconfig` set by default, so you just need to mount your kubeconfig to that path.

> **Note:** If `KUBECONFIG` is not set, use the default path:
> ```bash
> $CONTAINER_TOOL run --rm -ti \
>   -v ~/.kube/config:/kubeconfig \
>   quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
> ```

> **SELinux:** On SELinux systems (Fedora, RHEL, CentOS), add `:Z` to volume mounts:
> ```bash
> # With KUBECONFIG set
> $CONTAINER_TOOL run --rm -ti \
>   -v $KUBECONFIG:/kubeconfig:Z \
>   quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
>
> # With default kubeconfig path
> $CONTAINER_TOOL run --rm -ti \
>   -v ~/.kube/config:/kubeconfig:Z \
>   quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
> ```

**Working with Multiple Clusters:**

If your kubeconfig contains multiple cluster contexts, the CLI uses the `current-context`. You have two options:

**Option 1: Switch context before running (Recommended)**

```bash
# Switch to desired cluster
kubectl config use-context <context-name>

# Run the CLI (uses new context-name)
$CONTAINER_TOOL run --rm -ti \
  -v ~/.kube/config:/kubeconfig \
  quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
```

**Option 2: Set context with --context flag**

```bash
$CONTAINER_TOOL run --rm -ti \
  -v ~/.kube/config:/kubeconfig:Z \
  quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0 \
  --context <context-name>
```

**Available Tags:**
- `:dev` - Latest development build from main branch
- `:rhoai-X.Y-ea.Z` - Specific version (e.g., `:rhoai-3.4-ea.1`)

> **Note:** The images are OCI-compliant and work with both Podman and Docker.

**Interactive Shell Access:**

For interactive debugging and running upgrade helper scripts:

**Step 1: Login to cluster (on your host)**
```bash
# Login on your cluster which should  update ~/.kube/config
oc login --token=sha256~xxxx --server=https://api.my-cluster.example.com:6443
```

**Step 2: Shell into container**
```bash
# Mount the kubeconfig that was created by oc login
$CONTAINER_TOOL run -it --rm \
  -v ~/.kube/config:/kubeconfig \
  --entrypoint /bin/bash \
  quay.io/rhoai/odh-cli-rhel9:dev
```

**Step 3: Inside container - you're already authenticated**

Available tools:
- `odh-cli` - The CLI tool (at `/opt/odh-cli/bin/odh-cli`)
- `kubectl` / `oc` - Kubernetes/OpenShift client tools from OCP 4.19
- Upgrade helper scripts at `/opt/rhai-upgrade-helpers`
- Standard utilities: `jq`, `yq`, `python3`, `wget`, `curl`, `tar`, `gzip`, `bash`

Example workflow:
```bash
# Verify connection
oc get dsci

# Run lint command
odh-cli lint --target-version 3.3.0

# Example: Run upgrade helper script
cd /opt/rhai-upgrade-helpers
./ray/ray_cluster_migration.py backup
```

**Using Upgrade Helper Scripts:**

The lint command identifies upgrade issues and provides remediation steps that reference upgrade helper scripts. Follow this workflow:

**Step 1: Run lint to identify issues**
```bash
$CONTAINER_TOOL run --rm -ti \
  -v ~/.kube/config:/kubeconfig \
  quay.io/rhoai/odh-cli-rhel9:dev lint --target-version 3.3.0
```

**Step 2: Review remediation guidance**

Lint output shows which helper scripts to run:
```
CHECK: Ray workloads migration
REMEDIATION: Run ray_cluster_migration.py from rhoai-upgrade-helpers repository
```

**Step 3: Run helper scripts in shell mode**

Shell into the container with backup directory mounted:
```bash
$CONTAINER_TOOL run -it --rm \
  -v ~/.kube/config:/kubeconfig:Z \
  -v ./backup:/tmp/rhoai-upgrade-backup:Z \
  --entrypoint /bin/bash quay.io/rhoai/odh-cli-rhel9:dev

# Inside container - run the recommended script
cd /opt/rhai-upgrade-helpers
./ray/ray_cluster_migration.py backup
./trustyai/backup-metrics.sh
```

Scripts write backups to `/tmp/rhoai-upgrade-backup/<component>/` which persists to `./backup/` on your localhost via the volume mount.

**Token Authentication:**

For environments with token and server URL (no kubeconfig file):

```bash
$CONTAINER_TOOL run --rm -ti \
  quay.io/rhoai/odh-cli-rhel9:dev \
  lint --target-version 3.3.0 \
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

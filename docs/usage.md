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
- `version` - Display CLI version information

## As kubectl Plugin

Install the `kubectl-odh` binary to your PATH:

```bash
# Download from releases
# Place in PATH as kubectl-odh
# Use with kubectl
kubectl odh lint --target-version 3.3.0
kubectl odh version
```

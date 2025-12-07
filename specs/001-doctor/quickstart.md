# Quickstart Guide: Doctor Subcommand

**Feature**: 001-doctor | **Date**: 2025-12-06

## Overview

The `kubectl-odh doctor` command provides diagnostic capabilities for OpenShift AI clusters, helping administrators identify configuration errors, misconfigurations, and upgrade blockers.

This guide walks you through installation, basic usage, and common scenarios.

---

## Prerequisites

- OpenShift AI cluster (version 2.x or 3.x)
- `kubectl` installed and configured
- Read-only cluster access (minimum permissions: get, list, watch on relevant resources)

---

## Installation

### Option 1: Download Binary

```bash
# Download latest release
curl -LO https://github.com/lburgazzoli/odh-cli/releases/latest/download/kubectl-odh

# Make executable
chmod +x kubectl-odh

# Move to PATH
sudo mv kubectl-odh /usr/local/bin/

# Verify installation
kubectl plugin list | grep odh
```

### Option 2: Build from Source

```bash
# Clone repository
git clone https://github.com/lburgazzoli/odh-cli.git
cd odh-cli

# Build binary
make build

# Install to PATH
sudo cp bin/kubectl-odh /usr/local/bin/

# Verify installation
kubectl odh version
```

### Option 3: Run from Source (Development)

```bash
# Clone repository
git clone https://github.com/lburgazzoli/odh-cli.git
cd odh-cli

# Run directly without building
go run cmd/main.go doctor lint

# Or use make target
make run
```

---

## Quick Start

### 1. Validate Current Cluster

Run a full diagnostic scan of your OpenShift AI cluster:

```bash
kubectl odh doctor lint
```

**Expected Output:**
```
CHECK                              STATUS  SEVERITY  MESSAGE
component-dashboard-deployment-... Pass    -         Dashboard deployment is healthy
component-workbenches-crd-estab... Pass    -         Workbenches CRD is established
service-oauth-client-exists        Fail    Critical  OAuth client odh-dashboard not found
workload-notebook-resource-limits  Fail    Warning   3/5 notebooks missing resource limits

Summary: 2 Pass, 2 Fail (1 Critical, 1 Warning)
```

### 2. Focus on Specific Issues

Run only component checks:

```bash
kubectl odh doctor lint --checks=components
```

Run only Critical issues:

```bash
kubectl odh doctor lint --severity=critical
```

### 3. Check Upgrade Readiness

Assess if your cluster is ready to upgrade to version 3.0:

```bash
kubectl odh doctor upgrade --version=3.0
```

**Expected Output:**
```
UPGRADE READINESS: 2.10.0 → 3.0.0

CHECK                              STATUS  SEVERITY  MESSAGE
upgrade-api-version-deprecated     Fail    Critical  Notebook CR uses deprecated v1alpha1
upgrade-component-renamed          Fail    Warning   ModelMesh component renamed to ModelServing in 3.0
upgrade-storage-migration-required Pass    -         No storage migration needed

Summary: 1 Pass, 2 Fail (1 Critical, 1 Warning)
Recommendation: Address 1 Critical issue before upgrading
```

---

## Common Scenarios

### Scenario 1: Post-Installation Validation

After installing OpenShift AI, verify everything is configured correctly:

```bash
# Run all checks
kubectl odh doctor lint

# Export results for reporting
kubectl odh doctor lint --output=json > diagnostics-$(date +%Y%m%d).json
```

### Scenario 2: Troubleshooting Component Issues

Dashboard not accessible? Check dashboard-specific issues:

```bash
# Run all dashboard checks
kubectl odh doctor lint --checks=dashboard-*

# View detailed output
kubectl odh doctor lint --checks=dashboard-* --output=yaml
```

### Scenario 3: Pre-Upgrade Validation

Before upgrading from 2.10 to 3.0:

```bash
# Check upgrade readiness
kubectl odh doctor upgrade --version=3.0

# Export upgrade assessment
kubectl odh doctor upgrade --version=3.0 --output=json > upgrade-assessment.json

# Only show blocking issues
kubectl odh doctor upgrade --version=3.0 --severity=critical
```

### Scenario 4: Automated Monitoring

Integrate into CI/CD or monitoring systems:

```bash
# Run checks and fail pipeline on critical issues
kubectl odh doctor lint --output=json --fail-on-critical=true

# Example in GitLab CI
script:
  - kubectl odh doctor lint --output=json > results.json
  - if [ $? -ne 0 ]; then cat results.json; exit 1; fi
```

### Scenario 5: Namespace-Specific Checks

Check workloads in a specific namespace:

```bash
# Check only data-science-project namespace
kubectl odh doctor lint --namespace=data-science-project --checks=workloads
```

### Scenario 6: Selective Workload Validation

Check only notebook workloads:

```bash
# Run all notebook-related checks
kubectl odh doctor lint --checks=*notebook*

# Check across all namespaces
kubectl odh doctor lint --checks=workload-notebook-*
```

---

## Understanding Check Results

### Status Types

- **Pass**: Check completed successfully, no issues found
- **Fail**: Configuration issue detected (severity: Critical, Warning, or Info)
- **Error**: Check could not complete (permissions, network, missing resources)
- **Skipped**: Check not applicable (version mismatch, component disabled)

### Severity Levels

- **Critical**: Blocking issues requiring immediate action (e.g., component not running)
- **Warning**: Non-blocking problems needing attention (e.g., deprecated settings)
- **Info**: Optimization suggestions (e.g., resource limits not set)

### Remediation Hints

Each failed check includes actionable guidance:

```
CHECK: service-oauth-client-exists
STATUS: Fail
SEVERITY: Critical
MESSAGE: OAuth client odh-dashboard not found
REMEDIATION: Check OAuth client configuration:
  kubectl get oauthclient odh-dashboard -o yaml
```

---

## Advanced Usage

### Output Formats

**Table (Default):**
```bash
kubectl odh doctor lint
```

**JSON (for automation):**
```bash
kubectl odh doctor lint --output=json
```

**YAML (for readability):**
```bash
kubectl odh doctor lint --output=yaml
```

### Filter by Severity

**Only Critical:**
```bash
kubectl odh doctor lint --severity=critical
```

**Critical and Warning:**
```bash
kubectl odh doctor lint --severity=warning
```

### Check Selection Patterns

**By category:**
```bash
kubectl odh doctor lint --checks=components
kubectl odh doctor lint --checks=services
kubectl odh doctor lint --checks=workloads
```

**By pattern:**
```bash
# All dashboard checks
kubectl odh doctor lint --checks=dashboard-*

# All CRD-related checks
kubectl odh doctor lint --checks=*-crd-*

# Multiple patterns
kubectl odh doctor lint --checks=dashboard-*,workbenches-*
```

**By specific ID:**
```bash
kubectl odh doctor lint --checks=component-dashboard-deployment-exists
```

### Exit Code Control

**Fail only on Critical:**
```bash
kubectl odh doctor lint --fail-on-critical=true --fail-on-warning=false
```

**Fail on Warning or higher:**
```bash
kubectl odh doctor lint --fail-on-warning=true
```

---

## Troubleshooting

### Version Detection Failed

```bash
$ kubectl odh doctor lint
Warning: unable to detect cluster version from any source
Proceeding with generic checks only (some checks may be skipped)
```

**Solution:** Ensure DataScienceCluster or DSCInitialization CRs exist:
```bash
kubectl get datasciencecluster -A
kubectl get dscinitializations -A
```

### Permission Denied

```bash
$ kubectl odh doctor lint
Error: insufficient permissions to access cluster resources
Required permissions: get, list, watch on customresourcedefinitions
```

**Solution:** Verify your RBAC permissions:
```bash
kubectl auth can-i get customresourcedefinitions
kubectl auth can-i list deployments -n opendatahub
```

### Cluster Unreachable

```bash
$ kubectl odh doctor lint
Error: unable to connect to cluster
```

**Solution:** Verify kubeconfig and cluster connectivity:
```bash
kubectl cluster-info
kubectl get nodes
```

### No Checks Running

```bash
$ kubectl odh doctor lint --checks=invalid-pattern
No checks matched selector: invalid-pattern
```

**Solution:** Use valid check selectors:
```bash
# List available categories
kubectl odh doctor lint --help

# Use correct patterns
kubectl odh doctor lint --checks=components
kubectl odh doctor lint --checks=dashboard-*
```

---

## Integration Examples

### GitLab CI/CD

```yaml
validate-cluster:
  stage: test
  script:
    - kubectl odh doctor lint --output=json > diagnostics.json
    - if [ $? -ne 0 ]; then cat diagnostics.json; exit 1; fi
  artifacts:
    reports:
      junit: diagnostics.json
    when: always
```

### GitHub Actions

```yaml
- name: Run ODH Doctor
  run: |
    kubectl odh doctor lint --output=json --fail-on-critical=true
```

### Prometheus Alerting

Export JSON results and parse for alerting:

```bash
#!/bin/bash
RESULT=$(kubectl odh doctor lint --output=json)
CRITICAL_COUNT=$(echo $RESULT | jq '.summary.critical')

if [ $CRITICAL_COUNT -gt 0 ]; then
  # Send alert to Prometheus Alertmanager
  curl -X POST http://alertmanager:9093/api/v1/alerts -d "[...]"
fi
```

### Scheduled Health Checks

Run periodic diagnostics with cron:

```bash
# Add to crontab
0 */6 * * * kubectl odh doctor lint --output=json > /var/log/odh-diagnostics-$(date +\%Y\%m\%d-\%H\%M).json
```

---

## Best Practices

### Regular Health Checks

- Run `kubectl odh doctor lint` after installations and upgrades
- Schedule periodic checks (e.g., daily) to catch drift
- Monitor Critical findings and address promptly

### Pre-Upgrade Validation

- Always run `kubectl odh doctor upgrade --version=<target>` before upgrading
- Address all Critical findings before proceeding
- Review Warning findings for potential post-upgrade issues

### Export Results

- Save JSON output for historical comparison
- Track findings over time to identify patterns
- Use for compliance reporting and audit trails

### Selective Execution

- Use `--checks` to focus on specific areas during troubleshooting
- Run targeted checks to reduce scan time (60%+ faster)
- Filter by severity to prioritize high-impact issues

### Automation-Friendly

- Use `--output=json` for machine-parsable results
- Set appropriate `--fail-on-*` flags for CI/CD pipelines
- Parse JSON output with `jq` or similar tools

---

## Next Steps

- **View all checks:** Explore available checks and categories
- **Customize configuration:** Add custom validation rules to bundled configs
- **Integrate monitoring:** Connect doctor checks to your observability platform
- **Contribute:** Add new checks for your specific OpenShift AI use cases

---

## Resources

- **GitHub Repository:** https://github.com/lburgazzoli/odh-cli
- **Issue Tracker:** https://github.com/lburgazzoli/odh-cli/issues
- **OpenShift AI Docs:** https://docs.redhat.com/en/documentation/red_hat_openshift_ai
- **ODH Operator:** https://github.com/opendatahub-io/opendatahub-operator

---

## Example Workflow

Here's a complete workflow for maintaining a healthy OpenShift AI cluster:

```bash
# 1. Install kubectl-odh
curl -LO https://github.com/lburgazzoli/odh-cli/releases/latest/download/kubectl-odh
chmod +x kubectl-odh && sudo mv kubectl-odh /usr/local/bin/

# 2. Verify installation
kubectl odh version

# 3. Run initial diagnostics
kubectl odh doctor lint --output=json > initial-diagnostics.json

# 4. Check for Critical issues
kubectl odh doctor lint --severity=critical

# 5. Address findings using remediation hints
kubectl odh doctor lint --checks=service-oauth-client-exists --output=yaml

# 6. Re-run to verify fixes
kubectl odh doctor lint --severity=critical

# 7. Before upgrading, assess readiness
kubectl odh doctor upgrade --version=3.0

# 8. Address upgrade blockers
kubectl odh doctor upgrade --version=3.0 --severity=critical --output=yaml

# 9. After addressing issues, re-check
kubectl odh doctor upgrade --version=3.0

# 10. Proceed with upgrade if all clear
# (perform actual upgrade via operator)

# 11. Post-upgrade validation
kubectl odh doctor lint

# 12. Schedule regular checks
echo "0 2 * * * kubectl odh doctor lint --output=json > /var/log/odh-daily-$(date +\%Y\%m\%d).json" | crontab -
```

This workflow ensures continuous validation and proactive issue detection for your OpenShift AI environment.

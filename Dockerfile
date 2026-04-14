# Build stage - Go compilation only
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder

# Build arguments for cross-compilation
ARG TARGETOS
ARG TARGETARCH

# Switch to root for installation
USER root

# Install make (using yum for go-toolset image)
RUN yum install -y make && yum clean all

WORKDIR /workspace

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Enable Go toolchain auto-download to match go.mod version requirement
ENV GOTOOLCHAIN=auto
RUN go mod download

# Copy source code and Makefile
COPY . .

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build using Makefile with cross-compilation
RUN make build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE}

# Tools stage - Download binaries and clone repositories
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS tools

# Build arguments for architecture-specific downloads
ARG TARGETARCH

# Install git and curl for downloads
RUN microdnf install -y git tar gzip && microdnf clean all

# Clone upgrade helpers repository (configurable via build args)
ARG UPGRADE_HELPERS_REPO=https://github.com/red-hat-data-services/rhoai-upgrade-helpers.git
ARG UPGRADE_HELPERS_BRANCH=main

RUN git clone --depth 1 --branch ${UPGRADE_HELPERS_BRANCH} \
    ${UPGRADE_HELPERS_REPO} /opt/rhai-upgrade-helpers \
    && rm -rf /opt/rhai-upgrade-helpers/.git

# Clone must-gather repository (configurable via build args)
ARG MUST_GATHER_REPO=https://github.com/red-hat-data-services/must-gather.git
ARG MUST_GATHER_BRANCH=main

RUN git clone --depth 1 --branch ${MUST_GATHER_BRANCH} \
    ${MUST_GATHER_REPO} /tmp/must-gather \
    && mv /tmp/must-gather/collection-scripts /opt/must-gather \
    && rm -rf /tmp/must-gather

# Install kubectl with multi-arch support (latest stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) KUBE_ARCH="amd64" ;; \
        arm64) KUBE_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${KUBE_ARCH}/kubectl"; \
    chmod +x kubectl; \
    mv kubectl /usr/local/bin/kubectl

# Install OpenShift CLI (oc) with multi-arch support (latest stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) OC_ARCH="amd64" ;; \
        arm64) OC_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    curl -fsSL -o openshift-client.tar.gz \
        "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.17/openshift-client-linux-${OC_ARCH}-rhel9.tar.gz"; \
    tar -xzf openshift-client.tar.gz; \
    chmod +x oc; \
    mv oc /usr/local/bin/oc; \
    rm -f openshift-client.tar.gz kubectl README.md

# Install yq with multi-arch support
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) YQ_ARCH="amd64" ;; \
        arm64) YQ_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    YQ_VERSION="v4.44.6"; \
    curl -fsSL -o /usr/local/bin/yq \
        "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${YQ_ARCH}"; \
    chmod +x /usr/local/bin/yq

# Runtime stage - minimal base with only essential packages
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

# Set default KUBECONFIG path for container usage
# Users can override this with -e KUBECONFIG=<path> when running the container
ENV KUBECONFIG=/kubeconfig

# Install base utilities (jq, wget, python3, python3-pip)
RUN microdnf install -y \
    jq \
    wget \
    python3 \
    python3-pip \
    && microdnf clean all

# Python deps for ray_cluster_migration.py (kubernetes, PyYAML)
RUN python3 -m pip install --no-cache-dir \
    'kubernetes>=28.1.0' \
    'PyYAML>=6.0'

# Copy Go binary from builder stage
COPY --from=builder /workspace/bin/kubectl-odh /opt/rhai-cli/bin/rhai-cli

# Copy tools from tools stage
COPY --from=tools /usr/local/bin/kubectl /usr/local/bin/kubectl
COPY --from=tools /usr/local/bin/oc /usr/local/bin/oc
COPY --from=tools /usr/local/bin/yq /usr/local/bin/yq

# Copy repositories from tools stage
COPY --from=tools /opt/rhai-upgrade-helpers /opt/rhai-upgrade-helpers
COPY --from=tools /opt/must-gather /opt/must-gather

# Add rhai-cli to PATH
ENV PATH="/opt/rhai-cli/bin:${PATH}"

# Create backup directory for upgrade artifacts (world-writable with sticky bit
# so arbitrary UIDs can create subdirectories without permission errors)
RUN mkdir -p /tmp/rhoai-upgrade-backup && chmod 1777 /tmp/rhoai-upgrade-backup

# Create must-gather output directory
RUN mkdir -p /tmp/must-gather && chmod 1777 /tmp/must-gather

# Set entrypoint to rhai-cli binary
# Users can override with --entrypoint /bin/bash for interactive debugging
ENTRYPOINT ["/opt/rhai-cli/bin/rhai-cli"]

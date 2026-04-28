# Build stage - use native platform for builder to avoid emulation
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
    CGO_ENABLED=1 \
    GOEXPERIMENT=strictfipsruntime \
    GO_BUILD_TAGS="-tags strictfipsruntime" \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE}

# Clone upgrade helpers repository (configurable via build args)
ARG UPGRADE_HELPERS_REPO=https://github.com/red-hat-data-services/rhoai-upgrade-helpers.git
ARG UPGRADE_HELPERS_BRANCH=main

RUN git clone --depth 1 --branch ${UPGRADE_HELPERS_BRANCH} \
    ${UPGRADE_HELPERS_REPO} /opt/rhai-upgrade-helpers \
    && rm -rf /opt/rhai-upgrade-helpers/.git

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi:latest

# Build arguments for downloading architecture-specific binaries
ARG TARGETARCH

# Set default KUBECONFIG path for container usage
# Users can override this with -e KUBECONFIG=<path> when running the container
ENV KUBECONFIG=/kubeconfig

# Install base utilities (jq, wget, python3, python3-pip)
RUN yum install -y \
    jq \
    wget \
    python3 \
    python3-pip \
    && yum clean all

# Python deps for ray_cluster_migration.py (kubernetes, PyYAML)
RUN python3 -m pip install --no-cache-dir \
    'kubernetes>=28.1.0' \
    'PyYAML>=6.0'

# Install OpenShift CLI (oc) and kubectl with multi-arch support
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) OC_ARCH="amd64" ;; \
        arm64) OC_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    echo "Installing oc and kubectl for architecture: $OC_ARCH"; \
    curl -fsSL -o openshift-client.tar.gz \
        "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.19/openshift-client-linux-${OC_ARCH}-rhel9.tar.gz"; \
    tar -xzf openshift-client.tar.gz; \
    chmod +x oc kubectl; \
    mv oc /usr/local/bin/oc; \
    mv kubectl /usr/local/bin/kubectl; \
    rm -f openshift-client.tar.gz README.md

# Install yq with multi-arch support (stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) YQ_ARCH="amd64" ;; \
        arm64) YQ_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    echo "Installing yq for architecture: $YQ_ARCH"; \
    YQ_VERSION="v4.44.6"; \
    curl -fsSL -o /usr/local/bin/yq \
        "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${YQ_ARCH}"; \
    chmod +x /usr/local/bin/yq

# Copy binary from builder (cross-compiled for target platform)
COPY --from=builder /workspace/bin/odh-cli /opt/odh-cli/bin/odh-cli

# Add odh-cli to PATH
ENV PATH="/opt/odh-cli/bin:${PATH}"

# Copy upgrade helpers from builder
COPY --from=builder /opt/rhai-upgrade-helpers /opt/rhai-upgrade-helpers

# Create backup directory for upgrade artifacts (world-writable with sticky bit
# so arbitrary UIDs can create subdirectories without permission errors)
# This is path is created by https://github.com/red-hat-data-services/rhoai-upgrade-helpers
RUN mkdir -p /tmp/rhoai-upgrade-backup && chmod 1777 /tmp/rhoai-upgrade-backup

# Set entrypoint to odh-cli binary
# Users can override with --entrypoint /bin/bash for interactive debugging
ENTRYPOINT ["/opt/odh-cli/bin/odh-cli"]

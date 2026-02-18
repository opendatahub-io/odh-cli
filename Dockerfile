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
    yq \
    wget \
    python3 \
    python3-pip \
    && yum clean all

# Python deps for ray_cluster_migration.py (kubernetes, PyYAML)
RUN python3 -m pip install --no-cache-dir \
    'kubernetes>=28.1.0' \
    'PyYAML>=6.0'

# Install kubectl with multi-arch support (latest stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) KUBE_ARCH="amd64" ;; \
        arm64) KUBE_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    echo "Installing kubectl for architecture: $KUBE_ARCH"; \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/${KUBE_ARCH}/kubectl"; \
    chmod +x kubectl; \
    mv kubectl /usr/local/bin/kubectl

# Install OpenShift CLI (oc) with multi-arch support (stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) OC_ARCH="amd64" ;; \
        arm64) OC_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    echo "Installing oc for architecture: $OC_ARCH"; \
    curl -fsSL -o openshift-client.tar.gz \
        "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.17/openshift-client-linux-${OC_ARCH}-rhel9.tar.gz"; \
    tar -xzf openshift-client.tar.gz; \
    chmod +x oc; \
    mv oc /usr/local/bin/oc; \
    rm -f openshift-client.tar.gz kubectl README.md

# Copy binary from builder (cross-compiled for target platform)
COPY --from=builder /workspace/bin/kubectl-odh /opt/rhai-cli/bin/rhai-cli

# Add rhai-cli to PATH
ENV PATH="/opt/rhai-cli/bin:${PATH}"

# Copy upgrade helpers from builder
COPY --from=builder /opt/rhai-upgrade-helpers /opt/rhai-upgrade-helpers

# Set entrypoint to rhai-cli binary
# Users can override with --entrypoint /bin/bash for interactive debugging
ENTRYPOINT ["/opt/rhai-cli/bin/rhai-cli"]

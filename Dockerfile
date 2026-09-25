# Build stage - use native platform for builder to avoid emulation
FROM --platform=$BUILDPLATFORM registry.access.redhat.com/ubi9/go-toolset:1.26@sha256:0a4666f7a4eb0644c97a73cba198eb268691b270d97831822689e7a2088f87be AS builder

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
ARG CLI_NAME=odh-cli

# Build using Makefile with cross-compilation
RUN make build \
    CGO_ENABLED=1 \
    GOEXPERIMENT=strictfipsruntime \
    GO_BUILD_TAGS="-tags strictfipsruntime" \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE} \
    CLI_NAME=${CLI_NAME}

# Runtime stage
FROM registry.access.redhat.com/ubi9/ubi:latest@sha256:a4b9ec09b1e790a53ef25b7777c539976abe519248264298e5194dcbceac8c31

# Build arguments for downloading architecture-specific binaries
ARG TARGETARCH

# Set default KUBECONFIG path for container usage
# Users can override this with -e KUBECONFIG=<path> when running the container
ENV KUBECONFIG=/kubeconfig

# Install kubectl with multi-arch support (latest stable version)
RUN set -e; \
    ARCH=${TARGETARCH:-amd64}; \
    case "$ARCH" in \
        amd64) KUBE_ARCH="amd64" ;; \
        arm64) KUBE_ARCH="arm64" ;; \
        ppc64le) KUBE_ARCH="ppc64le" ;; \
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
        ppc64le) OC_ARCH="ppc64le" ;; \
        *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; \
    esac; \
    echo "Installing oc for architecture: $OC_ARCH"; \
    curl -fsSL -o openshift-client.tar.gz \
        "https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable-4.17/openshift-client-linux-${OC_ARCH}-rhel9.tar.gz"; \
    tar -xzf openshift-client.tar.gz; \
    chmod +x oc; \
    mv oc /usr/local/bin/oc; \
    rm -f openshift-client.tar.gz kubectl README.md

# Install jq for JSON processing in migration verification steps
RUN dnf install -y jq && dnf clean all

# Copy binary from builder (cross-compiled for target platform)
COPY --from=builder /workspace/bin/kubectl-odh /opt/rhai-cli/bin/rhai-cli

# Add rhai-cli to PATH
ENV PATH="/opt/rhai-cli/bin:${PATH}"

# Create backup directory for upgrade artifacts (world-writable with sticky bit
# so arbitrary UIDs can create subdirectories without permission errors)
RUN mkdir -p /tmp/rhoai-upgrade-backup && chmod 1777 /tmp/rhoai-upgrade-backup

# Set entrypoint to rhai-cli binary
# Users can override with --entrypoint /bin/bash for interactive debugging
ENTRYPOINT ["/opt/rhai-cli/bin/rhai-cli"]

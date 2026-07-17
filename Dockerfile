FROM golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH

# Must match GitHub repository name
ARG PROJECT_NAME="oci-registry-mirror"
ARG BUILD_DATE
ARG GIT_COMMIT
ARG VERSION

ENV CGO_ENABLED=0
ENV GOMODCACHE=/go/pkg/mod
ENV GOCACHE=/root/.cache/go-build
ENV VERSION=${VERSION}

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY pkg ./pkg

RUN mkdir -p /out
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
      -ldflags "\
        -s -w \
        -X github.com/ghdrope/go-version.Version=${VERSION} \
        -X github.com/ghdrope/go-version.GitCommit=${GIT_COMMIT} \
        -X github.com/ghdrope/go-version.BuildDate=${BUILD_DATE}" \
      -o /out/${PROJECT_NAME} \
      ./cmd  



FROM debian:trixie-backports

# Must match GitHub repository name
ARG PROJECT_NAME="oci-registry-mirror"
ARG VERSION

ENV VERSION=${VERSION}
# Avoid interactive prompts during apt installs
ENV DEBIAN_FRONTEND=noninteractive

# Strict requirements: Skopeo and Trivy
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        skopeo \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor -o /etc/apt/keyrings/trivy.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb generic main" | tee /etc/apt/sources.list.d/trivy.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends trivy \
    && update-ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/${PROJECT_NAME} /usr/local/bin/${PROJECT_NAME}

# ---- Execution permissions ----
RUN chmod +x /usr/local/bin/${PROJECT_NAME}

# ---- Entrypoint ----
ENTRYPOINT ["/usr/local/bin/oci-registry-mirror"]

# ---- CMD ----
CMD ["mirror"]
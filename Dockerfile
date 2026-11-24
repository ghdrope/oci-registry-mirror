# Binary name, usually the GitHub repository name
ARG PROJECT_NAME="oci-registry-mirror"

FROM golang:1.27 AS builder

ARG TARGETOS
ARG TARGETARCH
ARG PROJECT_NAME
# Empty for single component projects, otherwise builds cmd/<COMPONENT>
ARG COMPONENT=""
ARG VERSION="development"
ARG GIT_COMMIT="none"
ARG BUILD_DATE="unknown"

ENV CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -trimpath \
      -ldflags "-s -w \
        -X github.com/ghdrope/go-version.Version=${VERSION} \
        -X github.com/ghdrope/go-version.GitCommit=${GIT_COMMIT} \
        -X github.com/ghdrope/go-version.BuildDate=${BUILD_DATE}" \
      -o "/out/${PROJECT_NAME}${COMPONENT:+-${COMPONENT}}" \
      "./cmd/${COMPONENT}"


FROM debian:trixie-slim

ARG VERSION="development"
ENV VERSION=${VERSION}

# upgrade picks up Debian security fixes released after the base image was built
# Project requirements: skopeo (mirror) and trivy (scan). curl and gnupg are
# only needed to add the trivy repository, so they are removed afterwards;
# the version checks at the end fail the build if that removed anything the
# tools need.
RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get upgrade -y \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        skopeo \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://aquasecurity.github.io/trivy-repo/deb/public.key | gpg --dearmor -o /etc/apt/keyrings/trivy.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/trivy.gpg] https://aquasecurity.github.io/trivy-repo/deb generic main" > /etc/apt/sources.list.d/trivy.list \
    && apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends trivy \
    && DEBIAN_FRONTEND=noninteractive apt-get purge -y --auto-remove curl gnupg \
    && rm -rf /var/lib/apt/lists/* \
    && skopeo --version \
    && trivy --version

# Binary: /usr/local/bin/<PROJECT_NAME> or /usr/local/bin/<PROJECT_NAME>-<COMPONENT>
COPY --from=builder /out/ /usr/local/bin/

ENTRYPOINT ["/usr/local/bin/oci-registry-mirror"]
CMD ["mirror"]

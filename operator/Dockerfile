# syntax=docker/dockerfile:1.7
# Build context expected at the repo root (the parent of operator/) so
# we can copy both the operator module and the shared `events` package
# if/when controllers reference it. Build with:
#   docker build -f operator/Dockerfile -t xpos-operator:dev .

ARG GO_VERSION=1.23

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

COPY operator/go.mod operator/go.sum* ./operator/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd operator && go mod download

COPY operator ./operator

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd operator && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/xpos-operator \
        ./cmd

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /
COPY --from=build /out/xpos-operator /xpos-operator

# 8081 - health probes
# 8443 - controller-runtime metrics (HTTPS)
EXPOSE 8081 8443

USER 65532:65532
ENTRYPOINT ["/xpos-operator"]

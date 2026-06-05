# xpos operator

Kubernetes operator for the xpos tunneling service. Owns the control
plane: it reconciles `Tunnel` and `Agent` custom resources, places
tunnels onto data-plane relay pods (DaemonSet), and reconciles Gateway
API `HTTPRoute` / `TCPRoute` for ingress.

This module is intentionally kept separate from the relay/agent Go
module so that the runtime binaries don't drag in the k8s API surface.

## Layout

```
operator/
  PROJECT                       # kubebuilder marker
  go.mod                        # separate module
  cmd/main.go                   # manager entrypoint
  api/v1alpha1/                 # CRD Go types (Tunnel, Agent)
  internal/controller/          # reconcilers
  hack/boilerplate.go.txt       # codegen header
  config/                       # (generated) kustomize manifests
```

## First-time setup

From this directory:

```sh
go mod tidy
make manifests generate
make build
```

`manifests` and `generate` invoke `controller-gen` via `go run`, so no
binaries need to be installed.

## Status

Phase 2 scaffolding only. The reconcilers compile and wire into the
manager but contain `TODO` markers where the actual placement, Gateway
API route generation, and lease-watch logic must land. See the package
docstrings on the reconcilers and the project root README for the
overall design.

## Required cluster prerequisites

- Gateway API CRDs installed (`gateway.networking.k8s.io/v1`).
- A Gateway API implementation that supports `TCPRoute` if you intend
  to use TCP tunnels (Envoy Gateway, Contour, etc.).
- Coordination API (`coordination.k8s.io/v1`) — present in any
  in-support Kubernetes release.

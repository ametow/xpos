# XPOS Architecture and Implementation Guide

This document explains the XPOS project architecture, the Kubernetes-native design decisions, and the implementation details for developers who may be new to Kubernetes operator patterns.

## Table of Contents

1. [Project Overview](#project-overview)
2. [Kubernetes Concepts Primer](#kubernetes-concepts-primer)
3. [High-Level Architecture](#high-level-architecture)
4. [End-to-End Workflow](#end-to-end-workflow)
5. [Project Structure](#project-structure)
6. [Control Plane: The Operator](#control-plane-the-operator)
7. [Data Plane: The Relay](#data-plane-the-relay)
8. [Agent CLI](#agent-cli)
9. [Wire Protocol Evolution (v1 → v2)](#wire-protocol-evolution-v1--v2)
10. [TCPRoute Integration](#tcproute-integration)
11. [Deployment](#deployment)
12. [Development Workflow](#development-workflow)

---

## Project Overview

XPOS is a tunneling system that allows users to expose local services to the public internet. It consists of:

- **Agent CLI**: Runs on the user's machine, connects to the relay, and forwards local service traffic
- **Relay**: A server running in Kubernetes that accepts public connections and multiplexes them to connected agents
- **Operator**: A Kubernetes controller that manages the lifecycle of tunnels and integrates with Gateway API for routing

The project has been refactored from a traditional standalone server into a Kubernetes-native architecture with a control plane (operator) and data plane (relay StatefulSet).

---

## Kubernetes Concepts Primer

If you're new to Kubernetes, here are the key concepts used in XPOS:

### Custom Resource Definitions (CRDs)

Kubernetes extends its API through custom resources. XPOS defines two CRDs:

- **Agent**: Represents a connected agent session. Contains identity, session ID, and the relay pod it's assigned to.
- **Tunnel**: Represents a single tunnel (HTTP or TCP). Contains protocol, hostname, and status including public address and TCP port.

CRDs are like database tables that the Kubernetes API server can store and serve. The operator watches these resources and reacts to changes.

### Controllers and Reconciliation

A **controller** is a loop that watches Kubernetes resources and makes changes to move the actual state toward the desired state. This pattern is called **reconciliation**.

For example, when a Tunnel CR is created:
1. The controller notices the new resource
2. It looks up the referenced Agent
3. It creates an HTTPRoute or TCPRoute pointing to the relay
4. It updates the Tunnel status to `Active`

If the HTTPRoute is deleted, the controller recreates it. This self-healing is the core value of the operator pattern.

### StatefulSet

A **StatefulSet** is like a Deployment but with stable network identities. Each pod gets a stable hostname (`relay-0`, `relay-1`, etc.) and a persistent DNS record. XPOS uses this for the relay because:

- Each relay pod needs a stable identity for the heartbeat Lease
- Per-pod Services can reliably target specific pods
- Rolling upgrades are graceful (one pod at a time)

### Gateway API

Gateway API is a Kubernetes standard for ingress routing (like Ingress but more powerful). XPOS uses it to route public traffic to tunnels:

- **Gateway**: The entry point (e.g., `xpos-gateway`) with listeners on ports 80/443 and a range of TCP ports
- **HTTPRoute**: Routes HTTP requests by hostname to a backend Service
- **TCPRoute**: Routes TCP connections on a specific port to a backend Service

XPOS reconciles these resources automatically when tunnels are created.

### Lease

Kubernetes provides a coordination API with **Lease** resources. XPOS uses Leases for heartbeats:

- Each relay pod creates a Lease with its name
- The operator watches Leases; if a Lease expires, it assumes the relay is dead and garbage-collects the associated Agent CRs
- This is simpler than using pod status because Leases work even if the relay is outside the cluster

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Public Internet                          │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Gateway (Envoy/Gateway API)                                      │
│  - HTTP listener on port 80                                      │
│  - TCP listeners on allocated ports (30000-30099)                │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                               │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Control Plane: Operator (Deployment)                     │   │
│  │  - Watches Agent/Tunnel CRs                              │   │
│  │  - Reconciles HTTPRoutes/TCPRoutes                       │   │
│  │  - Allocates TCP ports                                    │   │
│  │  - Manages per-pod Services                              │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Data Plane: Relay StatefulSet (replicas=2)               │   │
│  │  ┌─────────────┐  ┌─────────────┐                        │   │
│  │  │ relay-0     │  │ relay-1     │                        │   │
│  │  │ - Lease     │  │ - Lease     │                        │   │
│  │  │ - yamux srv │  │ - yamux srv │                        │   │
│  │  │ - pub ln    │  │ - pub ln    │                        │   │
│  │  └─────────────┘  └─────────────┘                        │   │
│  └───────────────────────────────────────────────────────────┘   │
│                                                                   │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  Custom Resources                                          │   │
│  │  - Agent CRs (one per connected agent)                     │   │
│  │  - Tunnel CRs (one per tunnel)                             │   │
│  │  - HTTPRoutes/TCPRoutes (reconciled by operator)           │   │
│  │  - Per-pod Services (relay-0, relay-1, ...)                │   │
│  └───────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│  Agent CLI (on user's machine)                                   │
│  - Connects to relay via yamux client                            │
│  - Accepts streams, forwards to local service                    │
└─────────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Separate modules**: The operator is a separate Go module (`operator/`) to avoid dependency bloat in the relay/agent.
2. **Relay as single writer**: The relay creates Agent and Tunnel CRs; the operator only reconciles. This avoids race conditions.
3. **Lease-based heartbeats**: Simpler than pod status watching and works out-of-cluster.
4. **Per-pod Services**: Each relay pod gets its own Service so HTTPRoute backends can target specific pods.
5. **Gateway API for routing**: Standard Kubernetes ingress mechanism, not a custom load balancer.
6. **Yamux multiplexing**: Protocol v2 multiplexes all traffic over a single TCP connection, eliminating per-tunnel private listeners.

---

## End-to-End Workflow

### HTTP Tunnel Creation

1. **Agent connects**:
   - User runs: `xpos http 8080 --server relay.example.com:9876 --token $TOKEN`
   - Agent opens TCP connection to relay's event server (port 9876)
   - Agent sends `TunnelRequest` event with protocol=`http`, auth token, etc.

2. **Relay authenticates and creates CRs**:
   - Relay validates auth token with auth backend
   - Relay creates Agent CR with identity and session ID
   - Relay creates Tunnel CR with protocol=`http`, hostname=`user.xpos-it.com`
   - Relay creates Lease for heartbeat (in-cluster only)

3. **Operator reconciles**:
   - Operator sees new Tunnel CR
   - Looks up referenced Agent CR, finds assigned relay pod
   - Creates HTTPRoute with:
     - `parentRefs` pointing to the Gateway
     - Hostname match rule (`user.xpos-it.com`)
     - Backend reference to the per-pod Service for that relay pod
   - Updates Tunnel status: `phase=Active`, `publicAddr=user.xpos-it.com`

4. **Relay responds to agent**:
   - Relay initializes HTTP tunnel (no public listener needed for HTTP)
   - Relay wraps agent connection in yamux session
   - Relay sends `TunnelCreated` event back to agent

5. **Traffic flow**:
   - Public user accesses `http://user.xpos-it.com`
   - Gateway routes to HTTPRoute backend (per-pod Service)
   - Relay receives connection on shared HTTP gateway
   - Relay parses Host header, finds matching tunnel
   - Relay opens yamux stream to agent, sends `OpenStream` event
   - Agent accepts stream, dials local service (port 8080)
   - Traffic bridges bidirectionally

### TCP Tunnel Creation

TCP tunnels are similar but with an extra step for port allocation:

1. **Agent connects** and relay authenticates (same as HTTP)

2. **Relay creates Tunnel CR first** (before binding listener):
   - Relay creates Tunnel CR with protocol=`tcp`
   - Relay polls the CR until `status.tcpPort` is set by operator (≤30s timeout)

3. **Operator reconciles**:
   - Operator sees new Tunnel CR
   - Port allocator scans existing Tunnel CRs, picks lowest unused port in range (e.g., 30000)
   - Operator sets `status.tcpPort=30000`
   - Operator creates TCPRoute with:
     - `parentRefs` pointing to Gateway's TCP listener on port 30000
     - Backend reference to per-pod Service on port 30000
   - Updates Tunnel status: `phase=Active`, `publicAddr=user.xpos-it.com:30000`

4. **Relay binds to allocated port**:
   - Relay reads `status.tcpPort=30000` from CR
   - Relay initializes TCP tunnel listener on `:30000`
   - Relay wraps agent connection in yamux session
   - Relay sends `TunnelCreated` event

5. **Traffic flow**:
   - Public user connects to `user.xpos-it.com:30000`
   - Gateway routes to TCPRoute backend (per-pod Service:30000)
   - Relay receives connection on port 30000
   - Relay opens yamux stream to agent, sends `OpenStream` event
   - Agent accepts stream, dials local service
   - Traffic bridges bidirectionally

### Agent Disconnection / Garbage Collection

1. **Agent disconnects**:
   - yamux session closes
   - Relay stops renewing Lease (if in-cluster)
   - Relay deletes Agent and Tunnel CRs

2. **Operator GC** (if relay crashed):
   - Operator watches Leases
   - If Lease for a relay pod expires (30s), operator marks associated Agent CRs for deletion
   - Tunnel CRs are garbage-collected via owner references

---

## Project Structure

```
xpos/
├── cmd/                    # Entry points
│   ├── relay/
│   │   ├── main.go         # Relay binary entrypoint
│   │   └── Dockerfile      # Multi-stage distroless build
│   └── agent/
│       └── main.go         # Agent CLI entrypoint
├── relay/                  # Relay library packages
│   ├── xpos/               # Main server logic
│   │   └── server.go       # handleEventServer, handleHttpGateway
│   ├── tunnel/             # Tunnel implementations
│   │   ├── tunnel.go       # Tunnel interface
│   │   ├── tcptunnel.go    # TCP tunnel (yamux multiplex)
│   │   ├── httptunnel.go   # HTTP tunnel (shared gateway)
│   │   └── tcptunnel_test.go
│   ├── k8s/                # Kubernetes client abstraction
│   │   ├── client.go       # Client interface
│   │   ├── real.go         # In-cluster implementation
│   │   ├── noop.go         # Out-of-cluster no-op
│   │   └── real_test.go
│   ├── admin/              # Admin server (healthz, metrics)
│   ├── auth/               # Authentication client
│   ├── server/             # TCP server utilities
│   └── constants/          # Protocol constants
├── agent/                  # Agent library packages
│   ├── cmd/                # CLI logic (cobra)
│   ├── config/             # Configuration
│   └── handler/            # Stream handler (yamux client)
├── events/                 # Shared wire protocol
│   ├── events.go           # Event types (TunnelRequest, TunnelCreated, OpenStream)
│   └── events_test.go
├── operator/               # Separate Go module (control plane)
│   ├── api/v1alpha1/       # CRD types
│   │   ├── agent_types.go
│   │   ├── tunnel_types.go
│   │   └── groupversion_info.go
│   ├── internal/controller/
│   │   ├── agent_controller.go      # Reconciles Agent, watches Leases
│   │   ├── tunnel_controller.go     # Reconciles Tunnel, HTTPRoute, TCPRoute
│   │   ├── relaypod_controller.go   # Reconciles per-pod Services
│   │   └── tcp_port_allocator.go    # Stateless TCP port allocation
│   ├── cmd/
│   │   └── main.go         # Manager wiring, scheme registration
│   ├── config/             # Kustomize overlays
│   │   ├── crd/            # CRD manifests
│   │   ├── rbac/           # RBAC rules
│   │   ├── manager/        # Operator Deployment
│   │   ├── relay/          # Relay StatefulSet + Service
│   │   ├── gateway/        # Gateway example
│   │   └── samples/       # Example Agent/Tunnel CRs
│   ├── QUICKSTART.md       # Local cluster setup guide
│   └── Dockerfile
├── Makefile                # Build targets (build-relay, build-agent, image)
├── build.sh                # Cross-compile agent
└── go.mod                  # Root module (relay/agent/events)
```

---

## Control Plane: The Operator

The operator is built with `controller-runtime`, the standard Kubernetes operator framework.

### Manager Setup (`operator/cmd/main.go`)

```go
scheme := runtime.NewScheme()
_ = clientgoscheme.AddToScheme(scheme)
_ = xposv1alpha1.AddToScheme(scheme)
_ = gatewayv1.AddToScheme(scheme)
_ = gatewayv1a2.AddToScheme(scheme)  // For TCPRoute (v1alpha2)

mgr, _ := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
    Scheme:             scheme,
    Metrics:            metricsserver.Options{BindAddress: "0"},
    HealthProbeBindAddress: ":8081",
})

allocator := &controller.TCPPortAllocator{
    Client: mgr.GetClient(),
    Min:    30000,
    Max:    30099,
}

controller.SetupAgentReconciler(mgr)
controller.SetupTunnelReconciler(mgr, allocator)
controller.SetupRelayPodReconciler(mgr)
```

### Agent Reconciler (`agent_controller.go`)

**Watches**:
- Agent CRs (primary)
- Lease resources (for heartbeat expiry)

**Reconciliation logic**:
1. If Agent has no `relayPod` assignment, find an available relay pod
2. Update `spec.relayPod` with chosen pod
3. Set status phase to `Assigned`
4. Watch the relay pod's Lease; if Lease expires, delete the Agent CR

**Why**: This is the placement logic. When an agent connects, the relay creates an Agent CR. The operator picks a relay pod and assigns it. If the relay crashes (Lease expires), the operator GCs the Agent so tunnels don't point to dead pods.

### Tunnel Reconciler (`tunnel_controller.go`)

**Watches**:
- Tunnel CRs (primary)
- Agent CRs (to react to placement changes)

**Reconciliation logic**:
1. Resolve referenced Agent, get assigned relay pod
2. Update `status.assignedPod` and `status.publicAddr`
3. For HTTP: call `reconcileHTTPRoute`
4. For TCP: call `allocator.Allocate`, set `status.tcpPort`, call `reconcileTCPRoute`
5. Mark `phase=Active`

**HTTPRoute reconciliation**:
- Create HTTPRoute with hostname match rule
- Backend = per-pod Service (e.g., `relay-0`) on the relay's HTTP port
- Parent ref = configured Gateway

**TCPRoute reconciliation**:
- Create TCPRoute with sectionName matching the allocated port
- Backend = per-pod Service on the same port
- Parent ref = configured Gateway's TCP listener

### RelayPod Reconciler (`relaypod_controller.go`)

**Watches**:
- Pods with label `app.kubernetes.io/name=xpos-relay`

**Reconciliation logic**:
1. For each relay pod, create a Service named after the pod
2. Service selector = `statefulset.kubernetes.io/pod-name=<pod>`
3. This gives HTTPRoute/TCPRoute a stable backend target

**Why**: Gateway API backends need Services. Per-pod Services allow routing to specific relay pods, which is necessary for tunnel placement.

### TCP Port Allocator (`tcp_port_allocator.go`)

**Design**: Stateless scan of existing Tunnel CRs.

**Algorithm**:
1. If the tunnel already has `status.tcpPort` in range, return it (idempotent)
2. List all Tunnel CRs in the namespace
3. Build a set of used ports from `status.tcpPort`
4. Scan from Min to Max, return first unused port
5. If range exhausted, return error

**Why stateless?**: Simpler than in-memory bitmap with disk recovery. The source of truth is the CRs, so operator restart is safe. The scan is O(N) but N is expected to be small (hundreds of tunnels at most).

---

## Data Plane: The Relay

The relay is a Go server that runs as a Kubernetes StatefulSet. It has two main servers:

1. **Event server** (port 9876): Accepts agent connections, handles tunnel creation
2. **HTTP gateway** (port 80): Accepts public HTTP traffic, dispatches to tunnels by Host header

### Event Server (`relay/xpos/server.go:handleEventServer`)

Flow:
1. Accept TCP connection from agent
2. Read `TunnelRequest` event (protocol, auth token)
3. Authenticate with auth backend
4. Create Agent CR (in-cluster)
5. For TCP: create Tunnel CR, poll for `status.tcpPort` (≤30s)
6. Initialize tunnel (bind public listener for TCP; no-op for HTTP)
7. Write `TunnelCreated` event to agent
8. Wrap connection in yamux session
9. Start accept loop (for TCP public listener)
10. Block on yamux session close

### HTTP Gateway (`relay/xpos/server.go:handleHttpGateway`)

Flow:
1. Accept public HTTP connection
2. Call `parseHost` to extract Host header (buffered, reads until `\r\n\r\n` or 8KB)
3. Look up tunnel by hostname in `httpTunnels` map
4. Call `tunnel.PublicConnHandler(conn, buffer)` to bridge to agent

### TCP Tunnel (`relay/tunnel/tcptunnel.go`)

- **Init**: Binds public listener on the allocated port (or `:0` out-of-cluster)
- **Run**: Wraps agent connection in yamux server, starts accept loop
- **handlePublicConn**: For each public connection, opens yamux stream, writes `OpenStream` event, bridges

### HTTP Tunnel (`relay/tunnel/httptunnel.go`)

- No public listener (uses shared HTTP gateway)
- `PublicConnHandler` opens yamux stream, writes `OpenStream` event, bridges

### Kubernetes Client (`relay/k8s/`)

Abstraction layer so relay works out-of-cluster (no-op) and in-cluster (real client).

**Methods**:
- `Start`: Begin Lease renewal loop
- `CreateAgent`: Create Agent CR
- `DeleteAgent`: Delete Agent CR
- `CreateTunnel`: Create Tunnel CR
- `DeleteTunnel`: Delete Tunnel CR
- `WaitTunnelTCPPort`: Poll Tunnel CR until `status.tcpPort` is set
- `PodName`/`Namespace`: Return downward-API values

**Lease renewal**:
- Every 10s, update Lease `renewTime`
- Lease duration = 30s
- Operator watches Leases; if stale, GCs Agent CRs

---

## Agent CLI

The agent is a CLI tool built with cobra. It runs on the user's machine.

### Commands

- `xpos http <local_port> --server <relay> --token <token>`: Expose HTTP service
- `xpos tcp <local_port> --server <relay> --token <token>`: Expose TCP service

### Flow

1. Connect to relay's event server
2. Send `TunnelRequest` event
3. Wait for `TunnelCreated` event
4. Wrap connection in yamux client
5. Call `ServeStreams` to accept incoming streams
6. For each stream:
   - Read `OpenStream` event (contains client address)
   - Dial local service
   - Bridge stream and local connection bidirectionally

### Handler (`agent/handler/handler.go`)

- `ServeStreams`: Accept loop on yamux session
- `handleStream`: Reads `OpenStream`, dials local, bridges

---

## Wire Protocol Evolution (v1 → v2)

### Protocol v1 (deprecated)

- Each tunnel needed two ports:
  - Public listener (for incoming connections)
  - Private listener (agent dialed back for each connection)
- Agent opened a TCP listener on a random port
- Relay sent `NewConnection` event with private address
- Agent dialed back to private address for each public connection

**Problems**:
- Per-tunnel private listeners consume ports
- Dial-back adds latency and complexity
- Hard to firewall (agent must accept incoming connections)

### Protocol v2 (current)

- Single yamux session over the agent control connection
- No private listeners
- Relay opens yamux stream for each public connection
- Relay sends `OpenStream` event (contains client address)
- Agent accepts stream, dials local service

**Benefits**:
- Only one TCP connection per agent (control + multiplexed traffic)
- No dial-back latency
- Agent only needs outbound connectivity
- Simpler firewall rules

### Event Types

- **TunnelRequest**: Agent → Relay (protocol, auth token)
- **TunnelCreated**: Relay → Agent (hostname, public listener port, private listener port)
- **OpenStream**: Relay → Agent (client address, initial data)
- **Error**: Either direction (error message)

### Versioning

- First byte of connection is protocol version
- Version mismatch is rejected
- Currently at version 2

---

## TCPRoute Integration

TCP tunnels require a dedicated public port because Gateway API TCPRoute routing is port-based (unlike HTTP which is hostname-based).

### Port Allocation

The operator allocates ports from a configured range (default 30000-30099). The allocator is stateless:

1. Scan all Tunnel CRs, collect used ports from `status.tcpPort`
2. Pick lowest unused port in range
3. If tunnel already has a port in range, reuse it (idempotent)

### Relay Handshake

For TCP tunnels, the relay handshake order is different from HTTP:

1. Create Tunnel CR (without port)
2. Poll CR until `status.tcpPort` is set (≤30s timeout)
3. Initialize tunnel listener on that port
4. Send `TunnelCreated` event

This ensures the relay binds to the exact port the operator allocated, so the TCPRoute and the relay agree.

### TCPRoute Reconciliation

The operator creates a TCPRoute for each TCP tunnel:

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: tunnel-abc123-tcp
spec:
  parentRefs:
    - name: xpos-gateway
      sectionName: "30000"  # Matches Gateway listener
  rules:
    - backendRefs:
        - name: relay-0     # Per-pod Service
          port: 30000
```

The Gateway must have a TCP listener for each allocated port. In practice, this is either:
- A single Gateway with a wildcard TCP listener (if supported)
- Operator-managed Gateway with per-port listeners (current implementation assumes Gateway is pre-configured with a range)

---

## Deployment

### Local Cluster (Quickstart)

See `operator/QUICKSTART.md` for detailed steps:

1. Build images: `docker build -f cmd/relay/Dockerfile -t xpos-relay:dev .`
2. Deploy operator: `cd operator && make render | kubectl apply -f -`
3. Deploy Gateway: `kubectl apply -k config/gateway`
4. Verify heartbeats: `kubectl get leases`
5. Run agent: `go run ./agent http 8080 --server localhost:9876 --token $TOKEN`

### Production

Production deployment would typically:

- Use a real Gateway Class (Envoy Gateway, Contour, etc.)
- Configure TLS on the Gateway (cert-manager)
- Set up RBAC for the relay ServiceAccount
- Configure auth backend (OAuth, etc.)
- Set appropriate port ranges for TCP allocation
- Enable metrics and tracing

### Kustomize Overlays

The operator uses kustomize for deployment:

- `config/crd`: CRD manifests
- `config/rbac`: RBAC rules (ClusterRole, RoleBinding)
- `config/manager`: Operator Deployment
- `config/relay`: Relay StatefulSet + Service
- `config/gateway`: Gateway example
- `config/default`: Composes all overlays

---

## Development Workflow

### Building

```sh
# Build relay
make build-relay

# Build agent (cross-compile)
./build.sh

# Build operator
cd operator && make build
```

### Testing

```sh
# Root module (relay/agent/events)
go test ./...

# Operator module
cd operator && go test ./...
```

### Code Generation

The operator uses `controller-gen` for CRD and RBAC generation:

```sh
cd operator
make generate  # Generate deepcopy methods
make manifests  # Generate CRD manifests and RBAC
```

### Running Locally

**Relay**:
```sh
go run ./cmd/relay
```

**Agent**:
```sh
go run ./cmd/agent http 8080 --server localhost:9876 --token $TOKEN
```

**Operator** (against a remote cluster):
```sh
cd operator
make run
```

---

## Environment Variables

### Relay

- `XPOS_DOMAIN`: Base domain for hostnames (e.g., `xpos-it.com`)
- `XPOS_ADMIN_ADDR`: Admin server bind address (default `:8080`)
- `XPOS_POD_NAME`: Pod name (downward API, in-cluster)
- `XPOS_POD_NAMESPACE`: Pod namespace (downward API, in-cluster)

### Operator

- `XPOS_GATEWAY_NAME`: Gateway name for HTTPRoute/TCPRoute parentRefs
- `XPOS_GATEWAY_NAMESPACE`: Gateway namespace
- `XPOS_TCP_PORT_MIN`: Minimum TCP port for allocation (default 30000)
- `XPOS_TCP_PORT_MAX`: Maximum TCP port for allocation (default 30099)

---

## Future Work

- **Helm chart**: Alternative to kustomize for production deployments
- **Admission webhook**: Validate Tunnel CRs (hostname format, agentRef existence)
- **CI smoketest**: kind-based test that asserts `Tunnel.status.phase=Active`
- **Metrics integration**: Prometheus metrics for tunnel count, connection duration, etc.
- **TLS termination**: Support TLS on the Gateway and pass SNI to agents
- **UDP support**: Extend protocol to support UDP tunnels

---

## Summary

XPOS has been transformed from a standalone tunneling server into a Kubernetes-native system with:

- **Control plane**: Operator that manages tunnels via CRDs and Gateway API
- **Data plane**: Relay StatefulSet with yamux multiplexing and Lease heartbeats
- **Agent CLI**: Lightweight client that connects to relay and forwards local traffic

The architecture leverages standard Kubernetes patterns (controllers, CRDs, StatefulSets, Gateway API) for self-healing, scalability, and integration with the Kubernetes ecosystem. The wire protocol has been simplified (v2) to use yamux multiplexing, eliminating per-tunnel private listeners and reducing connection overhead.

For developers new to Kubernetes, the key takeaways are:

- **Controllers** are the brains: they watch resources and reconcile state
- **CRDs** extend the Kubernetes API with custom objects
- **StatefulSets** provide stable network identities for stateful workloads
- **Gateway API** is the modern standard for ingress routing
- **Leases** are a simple coordination primitive for heartbeats

The operator pattern makes XPOS resilient: if a relay crashes, the operator reassigns tunnels to healthy pods. If a route is deleted, the operator recreates it. This self-healing is the core value of Kubernetes-native design.

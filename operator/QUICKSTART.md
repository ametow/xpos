# xpos quickstart on a local cluster

End-to-end smoke test that brings up the operator + relay StatefulSet
on kind/k3d/minikube, confirms heartbeats, then drives a tunnel
through the agent CLI.

## 0. Prerequisites

- A Kubernetes cluster (1.29+).
- A Gateway API implementation. The simplest is **Envoy Gateway**:

  ```sh
  helm install eg oci://docker.io/envoyproxy/gateway-helm \
      --version v1.2.0 -n envoy-gateway-system --create-namespace
  ```

- `kubectl` 1.27+ with kustomize built in.
- `docker` (for building images) and `kind`/`k3d` if you don't have a
  cluster yet.

## 1. Build images

From the repo root:

```sh
# Relay (data plane)
docker build -f cmd/relay/Dockerfile -t xpos-relay:dev .

# Operator (control plane)
docker build -f operator/Dockerfile -t xpos-operator:dev .
```

If you're on `kind`, load them in:

```sh
kind load docker-image xpos-relay:dev xpos-operator:dev
```

## 2. Deploy the operator + relay

```sh
cd operator
make render | sed \
    -e 's|xpos-operator:latest|xpos-operator:dev|' \
    -e 's|xpos-relay:latest|xpos-relay:dev|' \
    | kubectl apply -f -
```

(or use `kustomize edit set image` if you prefer.)

## 3. Apply the Gateway

Edit `config/gateway/gateway.yaml` to match your installed
`GatewayClass`, then:

```sh
kubectl apply -k config/gateway
```

Wait for it to program:

```sh
kubectl -n xpos-system get gateway xpos-gateway -w
```

## 4. Verify the heartbeat path

After ~15 seconds you should see:

```sh
# Relay pods are Running and Ready.
kubectl -n xpos-system get pods -l app.kubernetes.io/name=xpos-relay

# Each relay pod has a Lease.
kubectl -n xpos-system get leases

# The operator created a per-pod Service for each relay pod.
kubectl -n xpos-system get svc -l 'app.kubernetes.io/name!=xpos-relay'
```

## 5. Run the agent against the relay

Port-forward the relay's event server, then run the agent locally
(adjust the auth token / domain to match your auth backend):

```sh
kubectl -n xpos-system port-forward svc/xpos-relay-headless 9876 &
go run ./agent http 8080 --server localhost:9876 --token $XPOS_TOKEN
```

While it runs, you should see CRs appear:

```sh
kubectl -n xpos-system get agents,tunnels
kubectl -n xpos-system get tunnels -o yaml | grep -E 'phase|publicAddr'
```

`status.phase` should reach `Active` and the operator should have
created an `HTTPRoute` named after the Tunnel:

```sh
kubectl -n xpos-system get httproutes
```

## 6. Tear down

```sh
kubectl delete -k config/gateway
make undeploy
```

## Troubleshooting

- **Tunnel stays Pending with `AgentNotFound`**: the relay isn't able
  to write CRs. Check the relay pod's logs and confirm the `relay`
  ServiceAccount has the binding from `config/relay/serviceaccount.yaml`.
- **Tunnel reaches Active but no traffic flows**: the per-pod Service
  might not be picking up the pod. Check that the StatefulSet's pod
  has the `statefulset.kubernetes.io/pod-name=<pod>` label (it's
  added automatically by Kubernetes 1.26+).
- **No HTTPRoute appears**: confirm `XPOS_GATEWAY_NAME` /
  `XPOS_GATEWAY_NAMESPACE` on the operator Deployment match your
  Gateway, and that the operator has RBAC for HTTPRoutes (it does
  by default; check the manager-role ClusterRole if customized).

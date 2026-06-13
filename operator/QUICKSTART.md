# xpos quickstart on a local cluster

End-to-end guide: create a kind cluster, build images, deploy the full
stack (operator + relay + Gateway), run a tunnel, and tear everything down.

---

## 0. Prerequisites

Install the required tools if not already present:

```sh
brew install kind kubectl helm go
brew install --cask docker
```

Verify:

```sh
docker version && kind version && kubectl version --client && helm version && go version
```

---

## 1. Create a kind cluster

```sh
kind create cluster --name xpos
kubectl config use-context kind-xpos
```

Verify the cluster is up:

```sh
kubectl get nodes
```

---

## 2. Install Gateway API CRDs

Must be applied before the operator, which registers resources that
reference Gateway API types:

```sh
kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
```

---

## 3. Install Envoy Gateway

```sh
helm install eg oci://docker.io/envoyproxy/gateway-helm \
    --version v1.2.0 -n envoy-gateway-system --create-namespace

kubectl -n envoy-gateway-system rollout status deploy/envoy-gateway
```

---

## 4. Build images

Run all commands from the **repo root**:

```sh
# Relay (data plane)
docker build -f cmd/relay/Dockerfile -t xpos-relay:dev .

# Operator (control plane)
docker build -f operator/Dockerfile -t xpos-operator:dev .
```

> **Note**: if the build fails on `FROM golang:1.26-alpine` (Go 1.26
> doesn't exist yet), change `GO_VERSION=1.23` in both Dockerfiles.

---

## 5. Load images into kind

`kind` has its own image cache separate from Docker — loading is required:

```sh
kind load docker-image xpos-relay:dev --name xpos
kind load docker-image xpos-operator:dev --name xpos
```

Verify the images are present inside the node:

```sh
docker exec -it xpos-control-plane crictl images | grep xpos
```

---

## 6. Deploy the operator + relay

The manifests reference `xpos-operator:latest` / `xpos-relay:latest` by
default. Substitute the `:dev` tags at apply time:

```sh
kubectl kustomize operator/config/default \
  | sed -e 's|xpos-operator:latest|xpos-operator:dev|g' \
        -e 's|xpos-relay:latest|xpos-relay:dev|g' \
  | kubectl apply -f -
```

This applies (in order): CRDs, RBAC, operator Deployment, relay
StatefulSet + ServiceAccount + Services — all in namespace `xpos-system`.

---

## 7. Apply the Gateway

```sh
kubectl apply -k operator/config/gateway
```

Wait for it to become programmed:

```sh
kubectl -n xpos-system get gateway xpos-gateway -w
```

---

## 8. Verify the stack

After ~15 seconds everything should be healthy:

```sh
# Operator pod
kubectl -n xpos-system get pods -l app.kubernetes.io/name=xpos-operator

# Relay pods (2 replicas)
kubectl -n xpos-system get pods -l app.kubernetes.io/name=xpos-relay

# Heartbeat Leases — one per relay pod, renewed every 10s
kubectl -n xpos-system get leases

# Per-pod Services created by RelayPodReconciler
kubectl -n xpos-system get svc
```

Expected state:

- `xpos-controller-manager-*` → `1/1 Running`
- `xpos-relay-0` and `xpos-relay-1` → `1/1 Running`
- Leases: `xpos-relay-0`, `xpos-relay-1`

---

## 9. Run the agent and verify a tunnel

```sh
# Port-forward the relay event server
kubectl -n xpos-system port-forward svc/xpos-relay-headless 9876 &

# XPOS_DEV_NO_AUTH=1 is set on the relay so any token works
go run ./cmd/agent http 8080 --server localhost:9876 --token test
```

Check CRs appear:

```sh
kubectl -n xpos-system get agents,tunnels
kubectl -n xpos-system get httproutes
kubectl -n xpos-system get tunnels -o yaml | grep -E 'phase|publicAddr'
```

`status.phase` should reach `Active`.

---

## 10. Tear down

### Option A — remove only the xpos workloads (keep the cluster)

```sh
# Remove the Gateway
kubectl delete -k operator/config/gateway

# Remove operator + relay + CRDs + RBAC
kubectl kustomize operator/config/default | kubectl delete --ignore-not-found -f -
# or: cd operator && make undeploy
```

### Option B — also remove Envoy Gateway

```sh
helm uninstall eg -n envoy-gateway-system
kubectl delete ns envoy-gateway-system
kubectl delete -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.0/standard-install.yaml
```

### Option C — delete the entire kind cluster (nuclear)

```sh
kind delete cluster --name xpos
```

Removes every namespace, CRD, and locally loaded image in one shot.

---

## Re-deploy cycle (after a code change)

```sh
# From repo root
docker build -f cmd/relay/Dockerfile -t xpos-relay:dev . \
  && kind load docker-image xpos-relay:dev --name xpos \
  && kubectl -n xpos-system rollout restart statefulset/xpos-relay
```

For the operator:

```sh
docker build -f operator/Dockerfile -t xpos-operator:dev . \
  && kind load docker-image xpos-operator:dev --name xpos \
  && kubectl -n xpos-system rollout restart deployment/xpos-controller-manager
```

---

## Troubleshooting

- **`ImagePullBackOff` on new pod**: the manifest tag (`:latest`) doesn't
  match what was loaded (`:dev`). Use the `sed` substitution in step 6,
  or re-tag before loading: `docker tag xpos-relay:dev xpos-relay:latest`.
- **Tunnel stays Pending with `AgentNotFound`**: the relay can't write CRs.
  Check the relay pod logs and confirm the `relay` ServiceAccount has the
  Role from `config/relay/serviceaccount.yaml`.
- **Tunnel reaches Active but no traffic flows**: the per-pod Service may
  not be selecting the pod. Confirm the pod has the
  `statefulset.kubernetes.io/pod-name=<pod>` label (auto-added by k8s 1.26+).
- **No HTTPRoute appears**: confirm `XPOS_GATEWAY_NAME` /
  `XPOS_GATEWAY_NAMESPACE` env vars on the operator Deployment match your
  Gateway (`xpos-gateway` / `xpos-system` by default).

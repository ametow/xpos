# Operator Makefile. Mirrors the conventions kubebuilder generates.
# All commands assume CWD = operator/.

CONTROLLER_TOOLS_VERSION ?= v0.16.5
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

VERSION ?= dev
IMAGE   ?= xpos-operator:$(VERSION)

.PHONY: all
all: build

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: manifests
manifests:
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook \
		paths="./..." \
		output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate:
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: build
build: generate fmt vet
	go build -o bin/xpos-operator ./cmd

.PHONY: image
image:
	docker build -f Dockerfile -t $(IMAGE) --build-arg VERSION=$(VERSION) ..

# `kubectl kustomize config/default` to render manifests; apply them
# with `make deploy` or pipe to `kubectl apply -f -`.
.PHONY: render
render:
	kubectl kustomize config/default

.PHONY: deploy
deploy:
	kubectl kustomize config/default | kubectl apply -f -

.PHONY: undeploy
undeploy:
	kubectl kustomize config/default | kubectl delete --ignore-not-found -f -

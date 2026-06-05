VERSION ?= dev
IMAGE   ?= xpos-relay:$(VERSION)

.PHONY: build-agent build-relay vet test image clean

build-agent:
	./build.sh

build-relay:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/xpos-relay ./cmd/relay

vet:
	go vet ./...

test:
	go test ./...

image:
	docker build -f cmd/relay/Dockerfile -t $(IMAGE) --build-arg VERSION=$(VERSION) .

clean:
	rm -rf bin/

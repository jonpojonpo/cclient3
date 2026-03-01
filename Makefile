BINARY := cclient3
GO := GOROOT=/usr/lib/go-1.24 PATH=/usr/lib/go-1.24/bin:$$PATH go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags "-s -w -X github.com/jonpo/cclient3/pkg/version.Version=$(VERSION) \
	-X github.com/jonpo/cclient3/pkg/version.GitCommit=$(COMMIT) \
	-X github.com/jonpo/cclient3/pkg/version.BuildDate=$(DATE)"

.PHONY: all build run test lint fmt clean deps

all: deps fmt build

build:
	$(GO) build $(LDFLAGS) -o $(BINARY) ./cmd/cclient3

run: build
	./$(BINARY)

test:
	$(GO) test -v -race -count=1 ./...

lint:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

clean:
	rm -f $(BINARY)
	rm -rf bin/

deps:
	$(GO) mod tidy

static:
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -a -extldflags "-static" -o $(BINARY) ./cmd/cclient3

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY)-linux-amd64 ./cmd/cclient3

build-mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(BINARY)-darwin-arm64 ./cmd/cclient3

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY)-windows-amd64.exe ./cmd/cclient3

build-all: build-linux build-mac build-windows

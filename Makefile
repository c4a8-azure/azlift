BINARY     := azlift
MODULE     := github.com/c4a8-azure/azlift
CMD        := ./cmd/azlift
GOFLAGS    := -trimpath
LDFLAGS    := -s -w

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    += -X $(MODULE)/pkg/config.Version=$(VERSION) -X $(MODULE)/pkg/config.BuildTime=$(BUILD_TIME)

.PHONY: all build test lint install clean fmt vet

all: build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)

test:
	go test ./... -race -coverprofile=coverage.out

lint:
	golangci-lint run ./...

install: build
	install -m 0755 bin/$(BINARY) $(shell go env GOPATH)/bin/$(BINARY)

clean:
	rm -rf bin/ coverage.out

fmt:
	gofmt -w .

vet:
	go vet ./...

GO      ?= go
VERSION := $(shell cat VERSION)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BRANCH  := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
DATE      := $(shell date -u +%Y%m%d-%H:%M:%S)
BUILDUSER := $(shell id -un)@$(shell hostname -s)

LDFLAGS := \
  -s -w \
  -X github.com/prometheus/common/version.Version=$(VERSION) \
  -X github.com/prometheus/common/version.Revision=$(COMMIT) \
  -X github.com/prometheus/common/version.Branch=$(BRANCH) \
  -X github.com/prometheus/common/version.BuildUser=$(BUILDUSER) \
  -X github.com/prometheus/common/version.BuildDate=$(DATE)

.PHONY: all build build-all test vet lint fmt

all: fmt vet lint build test

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter .

build-all:
	GOOS=linux  GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter-linux-amd64 .
	GOOS=linux  GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter-linux-arm64 .
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter-darwin-arm64 .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run --config .github/.golangci.yml ./...

fmt:
	$(GO) fmt ./...

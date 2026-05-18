GO      ?= go
VERSION := $(shell cat VERSION)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BRANCH  := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +%Y%m%d-%H:%M:%S)
BUILDUSER := $(shell id -un)@$(shell hostname -s)

LDFLAGS := \
  -X github.com/prometheus/common/version.Version=$(VERSION) \
  -X github.com/prometheus/common/version.Revision=$(COMMIT) \
  -X github.com/prometheus/common/version.Branch=$(BRANCH) \
  -X github.com/prometheus/common/version.BuildUser=$(BUILDUSER) \
  -X github.com/prometheus/common/version.BuildDate=$(DATE)

.PHONY: all build test vet lint fmt

all: vet build test

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o garmin_exporter .

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

lint:
	golangci-lint run --config .github/workflows/.golangci.yml ./...

fmt:
	$(GO) fmt ./...

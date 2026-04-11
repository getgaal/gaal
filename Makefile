DIST_DIR      := dist
BIN           := $(DIST_DIR)/gaal
GOBIN         := $(shell go env GOPATH)/bin
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS       := -ldflags "-X gaal/cmd.Version=$(VERSION) -X gaal/cmd.BuildTime=$(BUILD_TIME)"
LDFLAGS_R     := -ldflags "-X gaal/cmd.Version=$(VERSION) -X gaal/cmd.BuildTime=$(BUILD_TIME) -s -w -buildid= -extldflags '-static'"

# Sandbox directory — override with: make sandbox SANDBOX=/my/dir
SANDBOX  ?= $(shell mktemp -d /tmp/gaal-test-XXXXXX)

# Cross-compilation — override with: make build-cross GOOS=linux GOARCH=arm64
GOOS      ?= $(shell go env GOOS)
GOARCH    ?= $(shell go env GOARCH)
BIN_CROSS ?= $(DIST_DIR)/gaal-$(GOOS)-$(GOARCH)

.PHONY: all build build-cross run run-service test test-race coverage coverage-ci lint hooks clean tidy sandbox sandbox-service sandbox-status

all: build

## build: compile the binary
build:
	@mkdir -p $(DIST_DIR)
	go build $(LDFLAGS) -o $(BIN)-dev .
	$(BIN)-dev schema -f $(DIST_DIR)/schema.json

build-release:
	@mkdir -p $(DIST_DIR)
	go build -trimpath $(LDFLAGS_R) -o $(BIN) .
	$(BIN) schema -f $(DIST_DIR)/schema.json
	

## run: sync once with the example config (uses real HOME)
run: build
	./$(BIN) --config example.gaal.yaml sync

## run-service: run in daemon mode with example config (Ctrl-C to stop)
run-service: build
	./$(BIN) --service --interval 30s --config example.gaal.yaml

## sandbox: one-shot sync in an isolated /tmp directory (safe for testing)
sandbox: build
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN) --config example.gaal.yaml --sandbox $(SANDBOX) --verbose sync
	@echo ""
	@echo "All files written under: $(SANDBOX)"
	@find $(SANDBOX) -maxdepth 4 | sort

## sandbox-service: service mode in sandbox (Ctrl-C to stop)
sandbox-service: build
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN) --config example.gaal.yaml --sandbox $(SANDBOX) --verbose --service --interval 30s

## sandbox-status: status report in sandbox
sandbox-status: build
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN) --config example.gaal.yaml --sandbox $(SANDBOX) status

## test: run unit tests
test:
	go test ./...

## test-race: run unit tests with race detector (used in CI)
test-race:
	go test -race -timeout 5m ./...

## coverage: run tests with coverage — generates all reports in report/
##   report/coverage.html          — standard go tool cover (default)
##   report/coverage-golang.html   — gocov-html, golang theme
##   report/coverage-kit.html      — gocov-html, AdminKit theme
##   report/coverage-treemap.svg   — go-cover-treemap heatmap
coverage:
	@mkdir -p report
	go test ./internal/... -coverprofile=report/coverage.out
	go tool cover -func=report/coverage.out
	go tool cover -html=report/coverage.out -o report/coverage.html
	$(GOBIN)/gocov convert report/coverage.out | $(GOBIN)/gocov-html -t golang > report/coverage-golang.html
	$(GOBIN)/gocov convert report/coverage.out | $(GOBIN)/gocov-html -t kit    > report/coverage-kit.html
	$(GOBIN)/go-cover-treemap -coverprofile report/coverage.out > report/coverage-treemap.svg
	@echo ""
	@echo "Reports generated in report/:"
	@echo "  coverage.html          (go tool cover)"
	@echo "  coverage-golang.html   (gocov-html — golang theme)"
	@echo "  coverage-kit.html      (gocov-html — kit theme)"
	@echo "  coverage-treemap.svg   (go-cover-treemap)"

## lint: check formatting (gofmt) and run static analysis (go vet)
lint:
	@UNFORMATTED=$$(gofmt -l .); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "The following files are not gofmt formatted:"; \
		echo "$$UNFORMATTED"; \
		exit 1; \
	fi
	go vet ./...

## hooks: install repo git hooks (.githooks/) as core.hooksPath
hooks:
	git config core.hooksPath .githooks
	@echo "git hooks installed from .githooks/"

## tidy: tidy dependencies
tidy:
	go mod tidy

## coverage-ci: run tests with coverage for CI (no external tools required)
##   report/coverage.out          — raw coverage profile
##   report/coverage-summary.txt  — per-function summary
##   report/coverage.html         — HTML report
coverage-ci:
	@mkdir -p report
	go test -race -coverprofile=report/coverage.out -covermode=atomic ./internal/...
	go tool cover -func=report/coverage.out | tee report/coverage-summary.txt
	go tool cover -html=report/coverage.out -o report/coverage.html

## build-cross: cross-compile for a target platform
##   Override GOOS, GOARCH and optionally BIN_CROSS, e.g.:
##   make build-cross GOOS=linux GOARCH=arm64
build-cross:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath $(LDFLAGS_R) -o $(BIN_CROSS) .
	$(BIN_CROSS) schema -f $(DIST_DIR)/schema.json

## clean: remove build artefacts
clean:
	rm -rf dist/

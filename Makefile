DIST_DIR      := dist
BIN           := $(DIST_DIR)/gaal
GOBIN         := $(shell go env GOPATH)/bin
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS       := -ldflags "-X github.com/getgaal/gaal/cmd.Version=$(VERSION) -X github.com/getgaal/gaal/cmd.BuildTime=$(BUILD_TIME)"
LDFLAGS_R     := -ldflags "-X github.com/getgaal/gaal/cmd.Version=$(VERSION) -X github.com/getgaal/gaal/cmd.BuildTime=$(BUILD_TIME) -s -w -buildid= -extldflags '-static'"

# Sandbox directory — override with: make sandbox SANDBOX=/my/dir
# Simply-expanded (:=) on purpose: with ?= the $(shell mktemp) re-ran at
# every $(SANDBOX) reference, so the directory announced by the echo was
# not the one sync actually wrote to.
SANDBOX  := $(or $(SANDBOX),$(shell mktemp -d /tmp/gaal-test-XXXXXX))

# Cross-compilation — override with: make build-cross GOOS=linux GOARCH=arm64
GOOS      ?= $(shell go env GOOS)
GOARCH    ?= $(shell go env GOARCH)
BIN_CROSS ?= $(DIST_DIR)/gaal-$(GOOS)-$(GOARCH)

.PHONY: all build build-cross run run-service test test-race test-ci test-e2e test-e2e-cli coverage coverage-ci lint hooks clean tidy sandbox sandbox-service sandbox-status

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
#
# Runs testdata/sandbox.gaal.yaml, not example.gaal.yaml: the example is
# documentation and uses illustrative placeholder URLs (github.com/example/…)
# that deliberately do not resolve, so running it always exited non-zero.
# The sandbox config is fully offline and self-contained, so this target is
# a real smoke test that must exit 0.
sandbox: build sandbox-seed
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN)-dev --config testdata/sandbox.gaal.yaml --sandbox $(SANDBOX) --verbose sync
	@echo ""
	@echo "All files written under: $(SANDBOX)"
	@find $(SANDBOX) -maxdepth 6 | sort

# sandbox-seed builds the stub sub-repos the sandbox config's
# `roots: ["*"]` discovers: two directories carrying a .git marker, and one
# without so the run also demonstrates that non-repo siblings are skipped.
#
# These are created here rather than committed as fixtures because git
# refuses to track any path containing a .git component, so a
# testdata/.../.git/ directory silently would not survive a clone — and the
# sandbox would then quietly stop exercising discovery at all.
sandbox-seed:
	@mkdir -p $(SANDBOX)/workspace/service-a/.git
	@mkdir -p $(SANDBOX)/workspace/service-b/.git
	@mkdir -p $(SANDBOX)/workspace/not-a-repo
	@echo "No VCS marker here - roots: [\"*\"] must skip this directory." \
		> $(SANDBOX)/workspace/not-a-repo/README.md

## sandbox-service: service mode in sandbox (Ctrl-C to stop)
sandbox-service: build sandbox-seed
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN)-dev --config testdata/sandbox.gaal.yaml --sandbox $(SANDBOX) --verbose --service --interval 30s

## sandbox-status: status report in sandbox
sandbox-status: build
	@echo "Sandbox directory: $(SANDBOX)"
	./$(BIN) --config example.gaal.yaml --sandbox $(SANDBOX) status

## test: run unit tests
test:
	go test -count=1 ./...

## test-race: run unit tests with race detector (used in CI)
test-race:
	go test -race -count=1 -timeout 5m ./...

## test-ci: CI-only — race detector + JUnit XML report + GitHub annotations (requires gotestsum)
test-ci:
	@mkdir -p report
	$(GOBIN)/gotestsum --junitfile report/tests.xml --format github-actions -- -race -count=1 -timeout 5m ./...

## test-e2e: end-to-end Docker suite (Layer 1 — filesystem assertions only)
##   Builds gaal for linux/<host-arch> (amd64 on x86_64, arm64 on Apple Silicon),
##   builds the test image with a matching --platform, runs the suite.
##   Requires Docker on the host. Set GAAL_E2E_REBUILD=1 to force a rebuild.
##   Override the target arch with: make test-e2e GOARCH=amd64
##
##   When gotestsum is on PATH (CI sets this up via test-ci's install step),
##   a JUnit XML report is written to report/e2e-tests.xml. CI uploads it
##   as the e2e-test-results artifact alongside the unit test report.
test-e2e:
	@mkdir -p test/e2e/fixtures report
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) \
		go build -trimpath -o test/e2e/fixtures/gaal .
	@if [ -x "$(GOBIN)/gotestsum" ]; then \
		GAAL_E2E_PLATFORM=linux/$(GOARCH) \
			$(GOBIN)/gotestsum --junitfile report/e2e-tests.xml --format github-actions \
			-- -tags e2e -count=1 -timeout 15m ./test/e2e/...; \
	else \
		GAAL_E2E_PLATFORM=linux/$(GOARCH) \
			go test -tags e2e -count=1 -timeout 15m ./test/e2e/...; \
	fi

## test-e2e-cli: full e2e suite including the heavy CLI verification layer
##   Installs claude-code + codex into the image and exercises them
##   against the configs gaal wrote. Slower; meant for nightly / release CI.
##   Writes report/e2e-cli-tests.xml when gotestsum is available.
test-e2e-cli:
	@mkdir -p test/e2e/fixtures report
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) \
		go build -trimpath -o test/e2e/fixtures/gaal .
	@if [ -x "$(GOBIN)/gotestsum" ]; then \
		GAAL_E2E_CLI=1 GAAL_E2E_PLATFORM=linux/$(GOARCH) \
			$(GOBIN)/gotestsum --junitfile report/e2e-cli-tests.xml --format github-actions \
			-- -tags e2e -count=1 -timeout 30m ./test/e2e/...; \
	else \
		GAAL_E2E_CLI=1 GAAL_E2E_PLATFORM=linux/$(GOARCH) \
			go test -tags e2e -count=1 -timeout 30m ./test/e2e/...; \
	fi

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
##   Note: does not run the binary post-build (the schema target is dev-only
##   and would fail when the host can't execute the cross-compiled output).
build-cross:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath $(LDFLAGS_R) -o $(BIN_CROSS) .

## clean: remove build artefacts
clean:
	rm -rf dist/

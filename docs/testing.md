# Testing

The unit-test suite runs via `make test` and `make coverage`. This document
covers the end-to-end suite, which exercises gaal against real VCS backends
and real agent CLIs inside Docker.

## End-to-end tests

The e2e suite runs gaal inside a hermetic Docker container so it can clone
real repos, write to agent skill dirs, and merge MCP configs without
touching your real `$HOME`.

```bash
make test-e2e        # Layer 1: filesystem assertions only (fast, ~45s)
make test-e2e-cli    # Layer 2: also installs claude-code + codex CLIs and
                     # verifies the configs gaal writes are accepted
                     # (~2 min; runs nightly in CI, not on every PR)
```

Requires Docker on the host. The Makefile builds gaal for `linux/<host-arch>`
(amd64 on x86_64, arm64 on Apple Silicon) and forwards `--platform` to
docker so the binary and image always match. Override with
`make test-e2e GOARCH=amd64`.

## Cached fixture image

The fixture image (`alpine + git + mercurial + python3 + node`) is published
to ghcr on every Dockerfile change. Pull it once to skip the slow `apk` and
`npm install` on your first local run:

```bash
docker pull ghcr.io/getgaal/gaal-e2e:base-latest
docker tag  ghcr.io/getgaal/gaal-e2e:base-latest gaal-e2e:base-latest
make test-e2e   # reuses the cached base layers; only the binary COPY re-runs
```

## CI artifacts

CI uploads the JUnit report (`report/e2e-tests.xml`) plus
`docker logs`/`docker inspect` diagnostics on failure as workflow artifacts.

## Verbose mode

For interactive debugging, watch every `docker exec` invocation (banner) and
gaal's stdout/stderr stream live to the terminal:

```bash
GAAL_E2E_VERBOSE=1 go test -v -tags e2e -run TestVCS_GitBackend_CloneAndCheckout ./test/e2e/...
```

Off by default so the per-PR run stays clean. The captured `ExecResult`
fields are unchanged either way, so existing assertions keep working.

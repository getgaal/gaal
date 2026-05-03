# Dockerfile for gaal end-to-end tests.
#
# Builds a small Alpine image carrying the prerequisites the e2e suite
# exercises:
#   - git, mercurial, subversion, python3     — VCS backends + HTTP fileserver
#   - ca-certificates                         — TLS for skill cloning
#   - bash, coreutils                         — predictable shell semantics
#   - node + npm                              — agent CLIs (claude-code, codex)
#
# Breezy (the modern Bazaar fork) is intentionally NOT installed: pip
# install needs gcc + musl-dev + python3-dev + a Rust toolchain (the
# C extensions can be skipped but Rust ones cannot), which would balloon
# the image by ~200 MB to cover a backend with effectively no real-world
# adoption. TestVCS_BzrBackend stays documented-as-skipped — see the
# t.Skip body in test/e2e/vcs_other_test.go.
#
# Two stages:
#
#   `base`   apk + (optional) npm install. Cacheable across CI runs and
#            published as `ghcr.io/getgaal/gaal-e2e:base-latest` from main.
#            Local devs and CI both pull it as a cache source so the slow
#            apk + npm install layers are reused.
#
#   `final`  base + the gaal binary. The binary changes per build, so this
#            layer is the only one that re-runs in the per-PR loop.
#
# Versions are pinned to specific patches so an upstream publish cannot
# break CI overnight. Bump them deliberately (and re-run `make test-e2e`)
# rather than letting the floating tags drift.
FROM alpine:3.20.3 AS base

ARG INSTALL_AGENT_CLIS=0
ARG CLAUDE_CODE_VERSION=2.1.126
ARG CODEX_VERSION=0.128.0

RUN apk add --no-cache \
        bash \
        ca-certificates \
        coreutils \
        git \
        mercurial \
        nodejs \
        npm \
        py3-pip \
        python3 \
        subversion

# Optional: install the agent CLIs that the heavy CLI-verification layer
# uses. Skipped by default so layer-1 builds stay fast.
RUN if [ "$INSTALL_AGENT_CLIS" = "1" ]; then \
        npm install -g --no-fund --no-audit \
            "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
            "@openai/codex@${CODEX_VERSION}"; \
    fi

# Persistent container entrypoint. The Go test harness keeps one container
# alive per `go test` invocation and `docker exec`s into it for every test.
CMD ["sleep", "infinity"]

# Final stage adds the gaal binary on top of the cached base. Local devs
# can pull just the base via:
#   docker pull ghcr.io/getgaal/gaal-e2e:base-latest
#   docker tag  ghcr.io/getgaal/gaal-e2e:base-latest gaal-e2e:base-latest
# and `make test-e2e` will reuse those layers on its next build.
FROM base AS final

COPY gaal /usr/local/bin/gaal
RUN chmod +x /usr/local/bin/gaal

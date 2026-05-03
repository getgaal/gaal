# Dockerfile for gaal end-to-end tests.
#
# Builds a small Alpine image carrying the prerequisites the e2e suite
# exercises:
#   - git, ca-certificates                    — VCS + TLS for skill cloning
#   - bash, coreutils                         — predictable shell semantics
#   - node + npm                              — agent CLIs (claude-code, codex)
#
# The gaal binary itself is NOT installed by the agent CLI install step:
# the surrounding Makefile target compiles it for linux/amd64 and the test
# harness COPYs it into /usr/local/bin/gaal as the LAST layer so iterative
# rebuilds only re-do the cheap final step. The agent CLIs are gated behind
# a build arg so the cheap layer-1 image can skip the slow npm install when
# Layer 2 (`GAAL_E2E_CLI=1`) is not requested.
#
# Versions are pinned to specific patches so an upstream publish cannot
# break CI overnight. Bump these deliberately (and re-run `make test-e2e`)
# rather than letting the floating tags drift.
FROM alpine:3.20.3

ARG INSTALL_AGENT_CLIS=0
ARG CLAUDE_CODE_VERSION=2.1.126
ARG CODEX_VERSION=0.128.0

RUN apk add --no-cache \
        bash \
        ca-certificates \
        coreutils \
        git \
        nodejs \
        npm

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

# The gaal binary is copied last so editing the binary only invalidates
# this layer (apk + npm stay cached).
COPY gaal /usr/local/bin/gaal
RUN chmod +x /usr/local/bin/gaal

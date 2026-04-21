## Summary

<!-- What does this PR do? Why? Keep it to 2-3 sentences. -->

## Type of Change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor (no functional change)
- [ ] Documentation
- [ ] Chore / dependency update

## Related Issues

<!-- Link every issue this PR closes or relates to. -->

Closes #

## Checklist

<!-- CI runs: lint → build → test-ci → coverage-ci.
     Tick every box that applies before requesting review. -->

- [ ] `make lint` passes — `gofmt` formatting + `go vet` clean
- [ ] `make build` passes on Linux and macOS
- [ ] `make test-ci` passes — unit tests with race detector
- [ ] `make coverage-ci` run (if this PR adds or changes covered code)
- [ ] Every new function has at least one `slog.Debug` / `slog.DebugContext` call
- [ ] Documentation updated if user-facing behaviour changed
- [ ] No secrets or credentials are exposed in logs or config snippets

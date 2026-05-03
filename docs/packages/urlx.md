# `internal/urlx`

> URL validation and credential redaction. The single place that
> decides whether a URL is allowed in either of the two threat models
> (remote fetch vs. VCS clone), and the single place that strips
> userinfo before logging.

## Public API

| Symbol | Description |
|--------|-------------|
| `ValidateRemoteFetchURL(rawurl string) error` | Allowed schemes for HTTP fetches: `https://` (any host), `http://` (loopback only). Used by `mcp` and `vcs.archive`. |
| `ValidateRepoURL(rawurl string) error` | Allowed schemes for VCS clones: `https://`, `ssh://`, `git://`; `http://`, `svn://`, `bzr://` for loopback only; SCP-style git URLs (`user@host:path`); empty scheme = local path |
| `Redact(rawurl string) string` | Strip `userinfo` from the URL so it is safe to log or surface in errors |

## Threat model

| Concern | Defence |
|---------|---------|
| `file://`, `gopher://`, `dict://`, `ftp://` reaching `http.DefaultClient` | Both validators reject |
| SSRF to internal hosts (RFC1918, link-local, AWS IMDS at 169.254.169.254) | `http://` rejected unless loopback |
| Credentials in URLs leaking into logs / telemetry | `Redact` strips `userinfo` |
| Public-network `svn://` / `bzr://` (cleartext daemon protocols) | Allowed only for loopback (CI fixtures); widening requires explicit security review |

## Flow

```mermaid
flowchart TD
    subgraph ValidateRemoteFetchURL
      A1[rawurl] --> A2[url.Parse]
      A2 --> A3{scheme}
      A3 -- https --> OK1([allow])
      A3 -- http --> A4{loopback host?}
      A4 -- yes --> OK1
      A4 -- no --> ZH([reject http+nonloopback])
      A3 -- "" --> ZE([reject missing scheme])
      A3 -- other --> ZS([reject scheme])
    end

    subgraph ValidateRepoURL
      B1[rawurl] --> B2{SCP-style git?}
      B2 -- yes --> OK2([allow])
      B2 -- no --> B3[url.Parse]
      B3 --> B4{scheme}
      B4 -- https/ssh/git --> OK2
      B4 -- http/svn/bzr --> B5{loopback host?}
      B5 -- yes --> OK2
      B5 -- no --> ZL([reject])
      B4 -- "" --> OK2
      B4 -- other --> ZO([reject])
    end
```

## Loopback detection (`isLoopbackHost`)

Accepts: `localhost`, `127.0.0.0/8`, `::1`, bracketed IPv6 (`[::1]`),
optional trailing `:port`. The check is **literal** — no DNS lookup —
so it cannot race against the actual connection.

## Redaction

`Redact("https://user:secret@example.com/path?q=1#frag")` →
`"https://example.com/path?q=1#frag"`.

PR #114 routes every log line that mentions a URL through `Redact`.

## Consumers

| Caller | Validator used |
|--------|---------------|
| `internal/mcp/manager.go` (remote source) | `ValidateRemoteFetchURL` |
| `internal/core/vcs/archive.go` (HTTP archive) | `ValidateRemoteFetchURL` |
| `internal/core/vcs/git.go` (`Clone`) | `ValidateRepoURL` |
| `internal/core/vcs/{hg,svn,bzr}.go` (`Clone`) | `ValidateRepoURL` |
| Anywhere that logs a URL | `Redact` |

## Tests

`scheme_test.go` covers each rejection rule; `redact_test.go` exercises
the userinfo strip cases (with/without password, IPv6, query/fragment
preservation).

## Related

- [`packages/httpx.md`](httpx.md) — the HTTP client paired with these
  validators.

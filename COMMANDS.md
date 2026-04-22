# Security & License Audit Commands

Quick reference to reproduce the full dependency audit (CVE + licences + legal summary).

## Prerequisites

```sh
# Install govulncheck (system)
sudo apt install govulncheck
# or
sudo snap install govulncheck

# Install go-licenses
go install github.com/google/go-licenses@latest
```

## 1 — Active CVE scan (called code only)

```sh
govulncheck ./...
```

## 2 — Full CVE scan (including imported but not called)

```sh
govulncheck -show verbose ./...
```

## 3 — License report (all direct + indirect dependencies)

```sh
go-licenses report ./... 2>/dev/null | sort -t',' -k3 | column -t -s','
```

## 4 — One-liner: CVE + licences in a single pass

```sh
echo "=== CVE ===" && govulncheck ./... ; echo "" && echo "=== LICENSES ===" && go-licenses report ./... 2>/dev/null | sort -t',' -k3 | column -t -s','
```

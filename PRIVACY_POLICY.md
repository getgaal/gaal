# Privacy Policy

**Last updated:** 2026-04-11
**Data controller:** GMG Inc.
**Contact:** hello@getgaal.com

---

## Overview

gaal collects **optional, anonymous usage data** to understand adoption and
improve the tool. Telemetry is **opt-in only** — nothing is sent unless you
explicitly enable it during the first-run prompt.

---

## What we collect

When telemetry is enabled, gaal sends the following data to our self-hosted
[Plausible Analytics](https://plausible.io) instance:

| Data point | Example | Purpose |
|---|---|---|
| Command name | `sync`, `init`, `doctor` | Understand which features are used |
| gaal version | `0.1.0` | Inform release and support decisions |
| Operating system | `darwin`, `linux` | Prioritize platform support |
| Architecture | `arm64`, `amd64` | Prioritize platform support |
| Installed agent names | `claude,cursor` | Inform adapter roadmap |
| Error category (on failure) | `yaml_parse_error` | Identify systemic issues |

We also track a small set of milestone events:

| Event | When it fires |
|---|---|
| Install | First run after opting in |
| First Sync | First successful `sync` command |
| Migration | `migrate --to community` is invoked |
| Error | A command exits with an error |

---

## What we never collect

The following are **never** sent under any circumstances:

- Configuration file contents
- File paths, directory structures, or repo names
- Hostnames or machine identifiers
- Usernames, emails, or any personally identifiable information
- Skill source URLs
- Error messages, stack traces, or log output
- Cookies or persistent tracking identifiers

---

## How we collect it

All telemetry is sent via the [Plausible Events API](https://plausible.io/docs/events-api).
Plausible does not use cookies. Unique visitors are counted by hashing the
visitor's IP address and User-Agent header; the raw IP is **discarded after
hashing** and is never stored.

---

## Where data is stored

Our Plausible instance is self-hosted on **Hetzner in Germany**. No data
leaves the European Union. The source code for our Plausible deployment is
public: <https://github.com/getgaal/plausible>.

---

## Data retention

Raw events are discarded after processing. Only aggregate statistics are
retained (e.g., "42 users ran `sync` on 2026-04-10"). Aggregates cannot be
traced back to individual users.

---

## Your choices

### Opt-in prompt

On first run, gaal asks:

```
gaal can send anonymous usage pings to help us understand adoption.
No config contents, file paths, or identifiers are ever sent.
Enable? [y/N]
```

The default is **No**. Your choice is saved and you are never asked again.

### Disabling telemetry

You can disable telemetry at any time:

| Method | How |
|---|---|
| Config file | Set `telemetry: false` in `~/.config/gaal/config.yaml` |
| Environment variable | `export GAAL_TELEMETRY=0` |
| Standard signal | `export DO_NOT_TRACK=1` |

Environment variables take precedence over the config file. When either
variable is set, telemetry is disabled and the first-run prompt is skipped.

### Checking your status

Run `gaal doctor` to see whether telemetry is currently enabled and why:

```
Telemetry: enabled (config)
Telemetry: disabled (GAAL_TELEMETRY=0)
Telemetry: not configured (will prompt on next run)
```

---

## Your rights under GDPR

Because we do not store personal data (IPs are hashed and discarded, no
identifiers are retained), there is typically no personal data to access,
correct, or delete. If you believe we hold personal data about you, contact
us at **hello@getgaal.com** and we will respond within 30 days.

You have the right to:

- **Access** any personal data we hold about you
- **Rectification** of inaccurate data
- **Erasure** of your data
- **Restrict processing** of your data
- **Object** to processing
- **Lodge a complaint** with your local data protection authority

---

## Third-party subprocessors

| Subprocessor | Purpose | Location |
|---|---|---|
| Hetzner Online GmbH | Infrastructure hosting for Plausible instance | Germany, EU |

No other third parties receive telemetry data.

---

## Changes to this policy

We will update this document when our data practices change. Material changes
will be noted in release notes. The "Last updated" date at the top reflects
the most recent revision.

---

## Contact

For privacy questions or data subject requests:
**hello@getgaal.com**

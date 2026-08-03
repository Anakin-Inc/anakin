# Security Policy

## Trust Model

**The API is not safe to expose to an untrusted network without `API_KEY` set.** A caller
who can reach the port can submit scrape jobs and read every job result on the instance.

- `API_KEY` — when set, every `/v1` route requires it (`X-API-Key`, `Api-Key`, or
  `Authorization: Bearer <key>`). Unset means an open instance; set it before binding the
  port to anything but localhost. Domain config writes always require it.
- `CORS_ALLOW_ORIGINS` — which browser origins may read API responses. Defaults to the
  webapp dev server. Setting `*` lets any website a developer visits drive the instance
  and read the results.
- Scrape targets reach whatever the server can route to. Treat the server's network
  position as part of the attack surface.

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

We only provide security fixes for the latest release. Please upgrade to the latest version before reporting.

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Report via our **[Discord server](https://discord.com/invite/T57dHrdT8u)** with:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

## What to Expect

- **Acknowledgment** within 48 hours
- **Assessment** within 1 week
- **Fix** within 90 days (critical vulnerabilities prioritized)
- **Credit** in the release notes (unless you prefer to remain anonymous)

## Scope

The following are in scope:

- AnakinScraper server (`server/`)
- Browser service (`browser-service/`)
- Docker configuration
- Default configurations that could lead to security issues

The following are out of scope:

- Issues in third-party dependencies (report to the upstream project)
- Issues that require physical access to the server
- Social engineering attacks

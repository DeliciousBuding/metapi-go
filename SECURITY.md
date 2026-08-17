# Security Policy

Metapi Go is a self-hosted API gateway. We take security seriously and
appreciate responsible disclosure.

## Reporting a vulnerability

**Do not open a public issue for security vulnerabilities.**

Report privately using GitHub's **Security Advisory** flow:

1. Go to <https://github.com/DeliciousBuding/metapi-go/security/advisories/new>
   (or the repo's **Security → Report a vulnerability** tab).
2. Include as much as possible:
   - Affected version (image tag, `metapi --version`, or commit hash).
   - Deployment mode (Docker image / docker-compose / source build) and
     database (SQLite / PostgreSQL).
   - A minimal reproduction (steps, request samples — redact any real tokens).
   - Impact assessment, if you have one.

All reports are kept private until a fix is available.

## Response expectations

| Step | Target |
|:-----|:-------|
| Initial acknowledgement | Within 3 business days |
| Triage and severity assessment | Within 10 business days |
| Fix release for critical/high | As soon as possible, coordinated with the reporter |

We will keep the reporter informed of progress and credit them in the release
notes (unless anonymity is requested).

## Scope

- The `metapi` server itself (admin API, proxy layer, routing, auth, store).
- The embedded frontend (React SPA).
- The Docker image and build pipeline.

Out of scope: misconfiguration of your own deployment (weak `AUTH_TOKEN`,
exposed ports, etc.) — please check the deployment docs and harden your
environment. General questions belong in issues or discussions, not
advisories.

## Self-hosting reminder

This is a self-hosted product: all credentials and traffic live on your own
server. Do not expose the admin UI or proxy port to the public internet
without a reverse proxy, TLS, and strong `AUTH_TOKEN` / `PROXY_TOKEN` values.

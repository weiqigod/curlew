# Security Policy

Curlew handles credentials on a user's behalf: vault provider secrets, auth
tokens, request-signing keys, and whatever a collection's variables happen to
carry. A vulnerability here can expose someone else's secrets, not just this
repository's source.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting route, not a public issue:

**https://github.com/weiqigod/curlew/security/advisories/new**

This opens a private draft security advisory that only maintainers can see,
until you and they agree it should become public.

Do not report a vulnerability in a public issue, a pull request, or a discussion: use the private advisory route above.

## What to include

- The version you tested against (`curlew --version`), or the commit if you
  built from source
- Steps to reproduce, ideally as a minimal collection file with any real
  secrets redacted
- What you expected to happen, what happened instead, and why you believe it
  is a security issue rather than a bug

## Response

There is no formal SLA yet — this is a small, actively developed project —
but every report is read as it arrives and acknowledged before triage
begins.

## Scope

In scope: the `curlew` CLI itself (`cmd/`, `internal/`) and everything it
ships, including the scaffolded templates in `templates/` and the schemas in
`schemas/`.

The `src/` (.NET backend) and `web/` (dashboard) directories are frozen
platform code the CLI does not call — see
[docs/TECH_CHOICES.md](docs/TECH_CHOICES.md#repository-shape) — but a
vulnerability there is still in scope. Report it through the same private
route above.

# Email Templates

This directory holds the M14 transactional email templates that the SendGrid
Dynamic Templates pipeline ships. See `docs/SPECIFICATION.md:8924-8953` for
the authoritative inventory and CI flow.

## File-pair convention

Each template is two files:

- `<slug>.mjml` — responsive MJML markup compiled to HTML by `MjmlNetCompiler`
- `<slug>.json` — manifest sidecar with slug, subject, variables, and test_data

The manifest schema is defined at `docs/SPECIFICATION.md:8941-8953`.

## M14 inventory

| Slug | Subject | Variables |
|------|---------|-----------|
| `email_verification` | Verify your ApiTool email | `first_name`, `verification_url` |
| `auth_device_code` | Your ApiTool device-login code | `user_code`, `verification_url`, `expires_in_minutes` |
| `billing_receipt` | Your ApiTool receipt for `{{billing_period}}` | `first_name`, `billing_period`, `amount_total`, `invoice_url` |
| `billing_payment_failed` | Payment issue with your ApiTool subscription | `first_name`, `amount_total`, `update_payment_url`, `attempt_count` |
| `billing_subscription_canceled` | Your ApiTool subscription has been canceled | `first_name`, `tier`, `effective_date` |
| `account_security_alert` | Security alert on your ApiTool account | `first_name`, `event_time`, `event_ip`, `relogin_url` |

Spec citation: `docs/SPECIFICATION.md:8926-8933`.

## Variable allowlist

The manifest's `variables` block is the allowlist enforced by
`SendGridSmtpSender.SendTemplateAsync` at compose time
(`docs/SPECIFICATION.md:8941-8942`). Any variable name not listed is
rejected with `EmailTemplateVariableUnknownException` before any SendGrid
HTTP call. Adding a new variable requires updating both the manifest and the
spec table.

## Placeholder-copy convention

Every MJML file opens with the comment:

```xml
<!-- placeholder copy; product/design polish in a later non-M14 commit -->
```

This is asserted by `EmailTemplateInventoryManifestTests.Mjml_starts_with_placeholder_copy_header`.
M14 ships the infrastructure with syntactically-valid placeholder copy; product/design
polishes copy in a separate non-M14 commit per Open Decision #8.

## Forbidden slugs

`password_reset` and `trial_expiring` are explicitly NOT in the M14 inventory.
They ship with M16's password-reset endpoint slice and trial cron slice
respectively, per investigation Decision #10. The absence of these files is
asserted by `EmailTemplateInventoryTests.Forbidden_templates_are_absent`.

## Dev preview

Render any template locally without a SendGrid account:

```bash
dotnet run --project src/ApiTool.Backend -- dev email-preview <slug> > /tmp/preview.html
open /tmp/preview.html
```

Example:

```bash
dotnet run --project src/ApiTool.Backend -- dev email-preview billing_receipt > /tmp/preview-billing_receipt.html
```

## CI upload

Templates are uploaded to SendGrid Dynamic Templates by the
`.github/workflows/email-templates.yml` workflow. The workflow spins up a fake
SendGrid server (`scripts/fake-sendgrid.py`) in CI and asserts that each of the
six templates is uploaded and a per-slug template ID is emitted. In production,
the real `SENDGRID_API_BASE` (defaults to `https://api.sendgrid.com`) is used.

# ApiTool Self-Hosted Bundle

Run the full ApiTool stack on your own infrastructure using Docker Compose.

## Quick Start

```bash
# 1. Copy the example env file and set your secrets
cp .env.example .env
# Edit .env — change POSTGRES_PASSWORD and JWT_SIGNING_KEY at minimum

# 2. Start the stack
docker compose -f docker-compose.yml --env-file .env up -d --build

# 3. Wait for all containers to reach healthy state (~60 seconds)
docker compose ps

# 4. Verify health
curl http://localhost:5000/health
# Expected: {"status":"healthy","db":"connected","redis":"connected"}

# 5. Open the web UI
open http://localhost:3000
```

## Services

| Service    | Default Port | Image                         |
|-----------|-------------|-------------------------------|
| `backend` | 5000        | Built from `src/ApiTool.Backend/Dockerfile` |
| `web`     | 3000        | Built from `web/Dockerfile`   |
| `postgres`| internal    | Custom `postgres.Dockerfile` (postgres:16-alpine + contrib) |
| `redis`   | internal    | `redis:7-alpine`              |

Postgres and Redis are not exposed to the host by default. Override ports in `.env` if needed.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKEND_PORT` | `5000` | Host port for the backend API |
| `WEB_PORT` | `3000` | Host port for the web UI |
| `ASPNETCORE_ENVIRONMENT` | `Production` | ASP.NET Core environment name |
| `POSTGRES_DB` | `apitool` | Postgres database name |
| `POSTGRES_USER` | `apitool` | Postgres superuser name |
| `POSTGRES_PASSWORD` | — | **Required.** Change before first boot |
| `REDIS_PASSWORD` | `""` | Redis password (empty = no auth). Set for production |
| `JWT_SIGNING_KEY` | — | **Required.** Must be at least 32 bytes |
| `BACKEND_RUN_MIGRATIONS` | `1` | Set to `1` to run EF Core migrations on startup (required for Postgres) |
| `BOOTSTRAP_ADMIN_EMAIL` | `""` | First-boot admin email. Leave blank after first boot. |
| `BOOTSTRAP_ADMIN_PASSWORD` | `""` | First-boot admin password (>=12 chars). Leave blank after first boot. |

> **Security:** `POSTGRES_PASSWORD` and `JWT_SIGNING_KEY` must be changed before first boot.
> The Postgres image only reads `POSTGRES_PASSWORD` when the data directory is empty (first boot).
> To rotate the password on an existing volume, you must `down -v` first (destroys all data).

## Volumes

| Volume | Contents |
|--------|----------|
| `pg_data` | Postgres data directory |
| `redis_data` | Redis append-only log |
| `backend_data` | Backend SQLite database (`apitool.db`) |

Data survives `docker compose down` (no `-v`). Use `docker compose down -v` to remove all volumes and start fresh.

## Upgrade

```bash
# Pull the latest images and rebuild
docker compose -f docker-compose.yml --env-file .env pull
docker compose -f docker-compose.yml --env-file .env up -d --build
```

## Backup

Backup the Postgres data volume before upgrading:

```bash
docker run --rm \
  -v apitool-self-hosted_pg_data:/source \
  -v "$(pwd)/backup":/dest \
  alpine tar czf /dest/pg_data_$(date +%Y%m%d%H%M%S).tar.gz -C /source .
```

Backup the SQLite database:

```bash
docker run --rm \
  -v apitool-self-hosted_backend_data:/source \
  -v "$(pwd)/backup":/dest \
  alpine cp /source/apitool.db /dest/apitool_$(date +%Y%m%d%H%M%S).db
```

## Smoke Test

An automated smoke test is provided in `scripts/test-self-hosted.sh`. It starts the stack, verifies all services are healthy, and tears it down. It is opt-in to avoid adding 60+ seconds to every CI run:

```bash
APITEST_RUN_SELF_HOSTED=1 ./scripts/ci-local.sh
# or directly:
./scripts/test-self-hosted.sh
```

## Troubleshooting

**Backend not starting:**
```bash
docker compose logs backend
```

**Port conflict:** Set `BACKEND_PORT` or `WEB_PORT` in `.env` to different values:
```dotenv
BACKEND_PORT=8080
WEB_PORT=3001
```

**Postgres password rotation:** The Postgres image only reads `POSTGRES_PASSWORD` on first init. To use a new password on an existing volume, you must destroy the volume:
```bash
docker compose -f docker-compose.yml --env-file .env down -v
# Update POSTGRES_PASSWORD in .env
docker compose -f docker-compose.yml --env-file .env up -d --build
```

**Redis auth:** M5-015 ships a no-auth Redis. If you set `REDIS_PASSWORD`, you must also configure the Redis container to use `--requirepass`. This is a known limitation documented for resolution in a follow-up task.

**Running alongside the test stack:** The self-hosted bundle and the repo's `docker-compose.test.yml` share the same host ports (5000, 3000). Stop one before starting the other:
```bash
./scripts/test-stack.sh down
docker compose -f deploy/self-hosted/docker-compose.yml --env-file deploy/self-hosted/.env up -d
```

**Do not override the project name** with `-p`. The smoke test and volume verification rely on the default project name prefix `apitool-self-hosted`.

## First-Boot Admin Bootstrap

When `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD` are both set, the backend seeds an admin user on first startup:

- Creates a local user with role `admin` and a default organization (`Enterprise` tier).
- The user can log in at `POST /api/v1/auth/login` with their email and password.
- Bootstrap is **idempotent**: if the email already exists, it logs `"bootstrap: admin already exists, skipping"` and continues.
- After first boot, clear the `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD` from `.env` and restart.

**Exit codes:**
- Exit `3`: Invalid bootstrap config (e.g., password < 12 chars, email-only without password). Check `docker compose logs backend`.
- Exit `4`: EF migrations failed (e.g., Postgres unreachable). Verify Postgres is healthy and `POSTGRES_*` vars are correct.

**Login endpoint:**
```bash
curl -sS -X POST \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"YourPassword"}' \
  http://localhost:5000/api/v1/auth/login
# Returns: {"access_token":"...","token_type":"Bearer","expires_in":28800,"role":"admin"}
```

> **Note:** Email addresses are normalized to lowercase. `Admin@Example.com` and `admin@example.com` refer to the same account.

## Stripe Configuration

ApiTool supports two billing gateway modes, selected via `APITOOL__STRIPE__MODE`.

| Variable | Default | Description |
|---|---|---|
| `APITOOL__STRIPE__MODE` | `fake` | `fake` uses the deterministic in-memory gateway (no real billing). `live` uses the Stripe.Net SDK. |
| `APITOOL__STRIPE__APIKEY` | — | Required when `APITOOL__STRIPE__MODE=live`. Your Stripe secret key (`sk_live_...` or `sk_test_...`). |
| `APITOOL__STRIPE__APIBASE` | — | Optional. Override the Stripe API base URL. Leave unset in production; set to `http://stripe-mock:12111` in CI only. |

### App Configuration (required for billing portal)

| Variable | Default | Description |
|---|---|---|
| `APITOOL__APP__WEBAPPURL` | — | **Required.** Absolute public URL of the web app (e.g. `https://app.apitool.dev`). Used as the default `return_url` for Stripe billing portal sessions. |

The backend **fails at startup** (`OptionsValidationException`) when `APITOOL__APP__WEBAPPURL` is not set. You must provide this value even in non-Stripe (`fake`) mode, because the option is validated unconditionally.

Add to your `.env`:
```dotenv
APITOOL__APP__WEBAPPURL=https://app.example.com
```

The `POST /api/v1/subscriptions/billing-portal` endpoint redirects users to `${APITOOL__APP__WEBAPPURL}/billing` after their Stripe billing portal session ends. Callers may override this by passing a `return_url` in the request body.

The default `fake` mode creates deterministic checkout URLs — **no real billing occurs**. Switch to `live` only after configuring Stripe webhooks (required for subscription lifecycle events, landing in M14-009/010).

Starting with `APITOOL__STRIPE__MODE=live` without `APITOOL__STRIPE__APIKEY` set causes the backend to fail at startup with a clear error message.

### Proration Computation

In `live` mode, proration math is computed **server-side by Stripe** via the upcoming-invoice preview (`GET /v1/invoices/upcoming`). The returned `amount_due_now` reflects Stripe's day-prorated calculation based on the current billing period.

In `fake` mode, a deterministic full-month-delta approximation is used instead. Dollar amounts in `fake` mode will **not** match real Stripe proration; this is intentional and documented. The `fake` mode is suitable for development and integration testing; use Stripe test mode (`sk_test_...` with `APITOOL__STRIPE__MODE=live`) for accurate proration figures.

## SendGrid Configuration

ApiTool sends transactional email via SendGrid (verification, billing receipts,
account security alerts, etc.). The integration has two modes:

| Variable | Default | Description |
|---|---|---|
| `APITOOL__SENDGRID__MODE` | `fake` | `fake` is a no-op logger (dev/CI). `live` posts to SendGrid. |
| `APITOOL__SENDGRID__APIKEY` | — | Required when `MODE=live`. Your SendGrid API key (`SG.xxx`). |
| `APITOOL__SENDGRID__FROMEMAIL` | `noreply@apitool.dev` | Sender email address. |
| `APITOOL__SENDGRID__FROMNAME` | `ApiTool` | Sender display name. |
| `APITOOL__SENDGRID__TEMPLATES__<SLUG>` | — | One env var per template. Populated by the CI upload job at release time. |

Per-template env vars (one per slug in the M14 inventory):
- `APITOOL__SENDGRID__TEMPLATES__EMAIL_VERIFICATION`
- `APITOOL__SENDGRID__TEMPLATES__AUTH_DEVICE_CODE`
- `APITOOL__SENDGRID__TEMPLATES__BILLING_RECEIPT`
- `APITOOL__SENDGRID__TEMPLATES__BILLING_PAYMENT_FAILED`
- `APITOOL__SENDGRID__TEMPLATES__BILLING_SUBSCRIPTION_CANCELED`
- `APITOOL__SENDGRID__TEMPLATES__ACCOUNT_SECURITY_ALERT`

The default `fake` mode logs every send attempt — no real email is delivered.
Switch to `live` only after the M14 release CI pipeline has uploaded the
template HTML and populated the `TEMPLATES__*` secrets.

Starting with `APITOOL__SENDGRID__MODE=live` without `APITOOL__SENDGRID__APIKEY` set causes
the backend to fail at startup with a clear error message.

### Local development preview

Render any template to stdout without a SendGrid account:

```bash
dotnet run --project src/ApiTool.Backend -- dev email-preview email_verification
```

## Public JWKS Endpoint

The CLI verifies License JWTs offline using the public JWKS exposed at:

```
GET /api/v1/.well-known/jwks.json
```

This endpoint is **public** and **unauthenticated** by design (RFC 8615 well-known
URI convention). If you front the backend with a reverse proxy, ensure this path
is allowed through without authentication. Responses are cacheable for 3600 seconds
(`Cache-Control: public, max-age=3600`).

```bash
curl http://localhost:5000/api/v1/.well-known/jwks.json
# Returns: {"keys":[{"kty":"EC","crv":"P-256","kid":"...","use":"sig","alg":"ES256","x":"...","y":"..."}]}
```

## Signing-Key Generation (FileKeyProvider)

ApiTool uses ES256 JWT signing. In self-hosted mode (`APITOOL__KEYPROVIDER__MODE=file`), the backend generates and stores key material in `APITOOL__KEYPROVIDER__FILE__DIR` (default: `/app/data/keys/signing`).

### Initial Setup

1. Ensure the key directory is writable by the backend container user:
   ```bash
   mkdir -p /app/data/keys/signing
   chmod 700 /app/data/keys/signing
   ```
2. On first boot the backend auto-generates a `current` ES256 key pair:
   - Private key: `<Dir>/<kid>.pem` with permissions **0600**
   - Public key JWK: cached in the `signing_keys` table
3. Verify the key was generated:
   ```bash
   curl http://localhost:5000/internal/keys/active
   # Returns: {"kid":"prod-es256-202605-a3f4d2"}
   ```

### Required Environment Variables

Add to your `.env`:
```dotenv
APITOOL__KEYPROVIDER__MODE=file
APITOOL__KEYPROVIDER__ENV=prod
APITOOL__KEYPROVIDER__FILE__DIR=/app/data/keys/signing
```

### Emergency Key Rotation

If a signing key is compromised, use the rotation endpoint:
```bash
curl -sS -X POST \
  "http://localhost:5000/internal/keys/rotate?emergency=true" \
  -H "Content-Type: application/json" \
  -d '{"reason": "key compromise detected"}'
# Returns: {"kid":"prod-es256-YYYYMM-xxxxxx"} (the new active kid)
```

This immediately marks the current key as `revoked` and promotes the next staged key (or bootstraps a new one if no staged key exists). Tokens signed with the old key are immediately invalid.

> **Warning:** Emergency rotation invalidates all active sessions. Plan for a re-authentication event.

### Key Backup

Back up the key directory along with the database:
```bash
docker run --rm \
  -v /app/data/keys:/source \
  -v "$(pwd)/backup":/dest \
  alpine tar czf /dest/keys_$(date +%Y%m%d%H%M%S).tar.gz -C /source .
```

## GitHub App Registration (M14-016)

ApiTool posts CI check results to GitHub via the GitHub Checks API. This requires registering a
GitHub App and providing the private key to the backend.

> **GHES Not Supported.** GitHub Enterprise Server is explicitly out of scope (spec :8696).
> The backend hardcodes `api.github.com` as the API base; GHES endpoints are never called.

### 1. Register a GitHub App

1. Go to **GitHub → Settings → Developer settings → GitHub Apps → New GitHub App**.
2. Set the **GitHub App name** (your chosen slug, e.g. `apitool-checks`). Record the **App ID** shown on the App page after creation.
3. Set **Homepage URL** to your deployment URL.
4. Under **Webhook**, set the **Webhook URL** to `https://your-host/webhooks/github` (wired in M14-018). Leave **Active** unchecked until M14-018 is deployed.
5. Under **Repository permissions**, grant:
   - `Metadata`: Read-only
   - `Checks`: Read & write
6. Under **Where can this GitHub App be installed?** choose `Only on this account` (or `Any account` for multi-tenant).
7. Click **Create GitHub App**.

### 2. Generate and save the private key

1. On the App page, scroll to **Private keys** and click **Generate a private key**.
2. GitHub downloads a `.pem` file (PKCS#1 RSA private key, 2048-bit).
3. On the backend host, store it at a path of your choice (e.g. `/app/data/keys/github-app/apitool-checks.pem`) and restrict permissions:
   ```bash
   mkdir -p /app/data/keys/github-app
   mv ~/Downloads/apitool-checks.*.private-key.pem /app/data/keys/github-app/apitool-checks.pem
   chmod 600 /app/data/keys/github-app/apitool-checks.pem
   ```

### 3. Configure the backend

Add to your `.env`:

```dotenv
GITHUB_APP__APPID=<numeric App ID from step 1>
GITHUB_APP__SLUG=apitool-checks
GITHUB_APP__KEYPROVIDER__MODE=file
GITHUB_APP__KEYPROVIDER__FILE__PATH=/app/data/keys/github-app/apitool-checks.pem
```

### 4. Verify the configuration

After restarting the backend, probe the self-test endpoint (internal access only):

```bash
curl -sS http://localhost:5000/internal/github-app/jwt-self-test | jq .
# Expected:
# {
#   "alg": "RS256",
#   "typ": "JWT",
#   "iss": <your_app_id>,
#   "iat_offset_seconds": -60,
#   "exp_offset_seconds": 540,
#   "verified": true
# }
```

A `verified: true` response confirms the key loaded correctly and signing succeeded.

### 5. Install the App

Install the GitHub App on the repositories you want to receive check results for:
**GitHub App page → Install App → Select repositories → Install**.

Installation tokens (used for the actual API calls) are handled in M14-018.

### 6. App-Installation Runbook (M14-017)

Once the GitHub App is registered and the backend is configured (steps 1-5), end users
install the App on their org and link it to their ApiTool org via either the
dashboard-initiated flow or the webhook-first claim flow.

**Required env var (in addition to M14-016):**

```dotenv
APITOOL__GITHUBAPP__STATESIGNINGKEY=<32+ char random secret>
```

**Dashboard-initiated install flow:**

1. The user clicks "Connect GitHub" in the dashboard.
2. Dashboard fetches `GET /api/v1/integrations/github/install-url` (requires admin or owner role)
   which returns `{ install_url: "https://github.com/apps/<slug>/installations/new?state=<token>", state_expires_at: "..." }`.
3. The user installs on GitHub; GitHub redirects back to
   `GET /api/v1/integrations/github/callback?installation_id=...&state=...`.
4. Backend verifies the signed state token (15-min expiry, HMAC-SHA256), looks up `org_id`,
   and writes the `github_installations` row.
5. User is redirected to `{WebAppUrl}/billing?installed=true`.

**Webhook-first install flow** (the user installed via GitHub UI directly): the
`installation.created` event arrives at `/webhooks/github` (wired in M14-019); backend writes
a row with `org_id = NULL`. The user claims the install in the dashboard later via
`POST /api/v1/integrations/github/claim` (full implementation in M14-018).

**Daily reconciliation:** `GithubInstallationReconcilerHost` runs every 24 hours, calling
`GET /installation/repositories` for each non-deleted, non-suspended row, correcting any drift
between `repo_set` and the GitHub-side reality.

**Cross-tenant isolation:** The `github_installations` table enforces at most one active
installation per org via partial UNIQUE index `idx_github_installations_org`:
`UNIQUE (org_id) WHERE deleted_at IS NULL AND org_id IS NOT NULL`.
A second install attempt for the same org returns HTTP 409 `install_already_linked`.

### KMS mode (SaaS deployments)

For SaaS deployments, use Google Cloud KMS with an `RSA_SIGN_PKCS1_2048_SHA256` key:

```dotenv
GITHUB_APP__APPID=<numeric App ID>
GITHUB_APP__SLUG=apitool-checks
GITHUB_APP__KEYPROVIDER__MODE=kms
GITHUB_APP__KEYPROVIDER__KMS__KMSKEYID=projects/<project>/locations/<location>/keyRings/<ring>/cryptoKeys/<key>/cryptoKeyVersions/<version>
```

The KMS key must be type `RSA_SIGN_PKCS1_2048_SHA256` (HSM tier). RS256 is mandated by GitHub for App JWTs.

---

## Known Limitations (M5-015 / M5-016)

- Redis auth (`REDIS_PASSWORD`) is surfaced as an env var but not yet wired into the Redis container command. Set for future use.
- `pg_cron` is available as an extension (contrib is installed) but `shared_preload_libraries` is not configured. Deferred to a later task.

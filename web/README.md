# Curlew Web

SvelteKit-based web dashboard for Curlew's team tier.

## Requirements

- Node.js 22+
- npm 10+

## Development

```bash
# Install dependencies
npm install

# Start dev server (http://localhost:5173)
npm run dev

# Type-check
npm run check

# Lint
npm run lint

# Run unit tests
npm run test:unit

# Build for production
npm run build
```

## Environment Variables

Copy `.env.example` to `.env` and set:

```
PUBLIC_API_URL=http://localhost:5000   # Curlew backend URL
```

## Running E2E Tests

The E2E spec at `tests/e2e/org-results.spec.ts` requires a running backend + web stack.

### Option 1: Managed stack (recommended for CI)

```bash
# Build and start the docker-compose test stack, then run the spec
CURLEW_MANAGE_STACK=1 npm run test:e2e -- tests/e2e/org-results.spec.ts

# Tear down when done
cd .. && ./scripts/test-stack.sh down
```

### Option 2: Manual stack

Start the stack separately, then run Playwright against it:

```bash
# From repo root — start the stack
./scripts/test-stack.sh up

# Run E2E spec
cd web
WEB_BASE_URL=http://localhost:3000 npm run test:e2e -- tests/e2e/org-results.spec.ts

# Tear down
cd .. && ./scripts/test-stack.sh down
```

## Authentication in Tests

The E2E spec seeds an `access_token` cookie by minting a dev JWT via
`scripts/test-token.sh`. This simulates login until a real magic-link
endpoint exists in the backend (TODO: M4-auth). The dev JWT is signed
with the key from `src/ApiTool.Backend/appsettings.Development.json`.

## Tier Gating

The `/org/[slug]/results` page requires the team tier. The backend does
not yet expose a `tier` field on `OrganizationDto` — absent tier is
treated as `'team'`. Non-team-tier tests use Playwright route
interception to return `tier: 'professional'` without needing a real
non-team org. See `TODO(M4-010)` in `src/lib/server/guards.ts`.

## Seeding a Team-Tier Org for Local Dev (Billing & Members)

The `scripts/seed-test-data.sh` script provisions a team-tier org (`acme`)
with owner (`owner@example.com`) and member (`member@example.com`) accounts.
Run it once after the backend starts:

```bash
# Start the backend (from repo root)
./scripts/test-stack.sh up

# Seed the test org (idempotent — safe to re-run)
bash scripts/seed-test-data.sh

# Mint dev tokens for local browser testing
bash scripts/test-token.sh owner@example.com   # copy token
bash scripts/test-token.sh member@example.com  # copy token
```

### Exercising billing locally

1. Open the browser to `http://localhost:3000/org/acme/billing`.
2. Paste the owner JWT as the value of an `access_token` cookie (DevTools →
   Application → Cookies) or use the `seedAuthCookie` helper in a Playwright
   context.
3. The billing page calls `GET /api/v1/subscriptions?org_id=<id>` backed by
   `FakeStripeGateway` in development — it returns deterministic data without a
   real Stripe key.

### Exercising members locally

1. Navigate to `http://localhost:3000/org/acme/members` with the owner token.
2. Use the **Invite member** button to POST to
   `/api/v1/organizations/{id}/invitations`.
3. Accept invitations via the raw token returned in the API response
   (`POST /api/v1/invitations/accept` with `{ token }`).

### Role guards summary

| Route | Minimum role |
|-------|-------------|
| `/org/[slug]/billing` | owner |
| `/org/[slug]/members` | admin |
| `/org/[slug]/settings/notifications` | admin |
| `/org/[slug]/results` | member |

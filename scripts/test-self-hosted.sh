#!/usr/bin/env bash
# test-self-hosted.sh — Brings up deploy/self-hosted, verifies the bundle is
# healthy end-to-end, and tears it down. Opt-in from ci-local.sh via
# APITEST_RUN_SELF_HOSTED=1 (stack boot is ~60 s and we don't want to pay that
# on every PR).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUNDLE="$REPO_ROOT/deploy/self-hosted"
cd "$BUNDLE"

# Guard: require docker.
command -v docker >/dev/null || { echo "docker is required" >&2; exit 2; }

trap 'docker compose -f docker-compose.yml --env-file .env down -v >/dev/null 2>&1 || true; rm -f .env' EXIT

cp .env.example .env

echo "=== Self-Hosted: up ==="
docker compose -f docker-compose.yml --env-file .env up -d --build

echo "--- Waiting for backend /health ---"
for i in $(seq 1 90); do
  if curl -fsS http://localhost:5000/health 2>/dev/null | grep -q '"status":"healthy"'; then
    echo "backend healthy after ${i}s"; break
  fi
  sleep 1
  if [ "$i" -eq 90 ]; then
    docker compose -f docker-compose.yml --env-file .env logs --tail 200 >&2
    echo "FAIL: backend did not reach healthy within 90s" >&2; exit 1
  fi
done

echo "--- /health reports db + redis connected ---"
BODY=$(curl -fsS http://localhost:5000/health)
echo "$BODY" | grep -q '"db":"connected"'    || { echo "FAIL: db not connected — $BODY" >&2; exit 1; }
echo "$BODY" | grep -q '"redis":"connected"' || { echo "FAIL: redis not connected — $BODY" >&2; exit 1; }
echo "PASS: /health healthy"

echo "--- web / returns title ---"
curl -fsS http://localhost:3000/ | grep -q '<title>apitest</title>' \
  || { echo "FAIL: web title missing" >&2; exit 1; }
echo "PASS: web title"

echo "--- Volume survives down (no -v) ---"
docker compose -f docker-compose.yml --env-file .env down
docker volume ls --format '{{.Name}}' | grep -q apitool-self-hosted_pg_data \
  || { echo "FAIL: pg_data missing after down" >&2; exit 1; }
docker volume ls --format '{{.Name}}' | grep -q apitool-self-hosted_redis_data \
  || { echo "FAIL: redis_data missing after down" >&2; exit 1; }
docker volume ls --format '{{.Name}}' | grep -q apitool-self-hosted_backend_data \
  || { echo "FAIL: backend_data missing after down" >&2; exit 1; }
echo "PASS: volumes survived"

echo "--- down -v removes volumes ---"
docker compose -f docker-compose.yml --env-file .env up -d
# Give services a moment to start so down -v has running containers to remove.
sleep 5
docker compose -f docker-compose.yml --env-file .env down -v
! docker volume ls --format '{{.Name}}' | grep -q apitool-self-hosted_pg_data \
  || { echo "FAIL: pg_data still present after down -v" >&2; exit 1; }
echo "PASS: down -v removed volumes"

echo "--- Bootstrap: first-boot admin login ---"
cp .env.example .env
# Append bootstrap credentials
cat >> .env <<'EOF'
BOOTSTRAP_ADMIN_EMAIL=admin@example.com
BOOTSTRAP_ADMIN_PASSWORD=ChangeMe!Password
EOF

docker compose -f docker-compose.yml --env-file .env up -d --build

echo "--- Waiting for bootstrapped backend /health ---"
for i in $(seq 1 90); do
  if curl -fsS http://localhost:5000/health 2>/dev/null | grep -q '"status":"healthy"'; then
    echo "backend healthy after ${i}s"; break
  fi
  sleep 1
  if [ "$i" -eq 90 ]; then
    docker compose -f docker-compose.yml --env-file .env logs --tail 200 >&2
    echo "FAIL: backend did not reach healthy within 90s" >&2; exit 1
  fi
done

BODY=$(curl -sS -X POST -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"ChangeMe!Password"}' \
  http://localhost:5000/api/v1/auth/login)
echo "$BODY" | grep -q '"access_token"' \
  || { echo "FAIL: no access_token in login response — $BODY" >&2; exit 1; }
echo "$BODY" | grep -q '"role":"admin"' \
  || { echo "FAIL: role != admin in login response — $BODY" >&2; exit 1; }
echo "PASS: bootstrap admin login"

echo "--- Bootstrap: idempotent on restart ---"
docker compose -f docker-compose.yml --env-file .env restart backend
sleep 15
docker compose -f docker-compose.yml --env-file .env logs backend \
  | grep -q 'bootstrap: admin already exists, skipping' \
  || { echo "FAIL: idempotent skip log missing" >&2; exit 1; }
echo "PASS: bootstrap idempotent"

docker compose -f docker-compose.yml --env-file .env down -v

echo "=== Self-Hosted smoke PASS ==="

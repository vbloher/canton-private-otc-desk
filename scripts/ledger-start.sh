#!/usr/bin/env bash
# Starts a local, in-memory Canton sandbox with the token-standard test rail and the demo seed.
# Keep it running in its own terminal; Ctrl-C stops the sandbox.
set -euo pipefail

cd "$(dirname "$0")/../daml"

JSON_API=${JSON_API:-http://localhost:6864}

if curl -fs "$JSON_API/readyz" >/dev/null 2>&1; then
  echo "ledger-start: a sandbox is already running on $JSON_API — stop it first (Ctrl-C in its terminal)" >&2
  exit 1
fi

dpm build --all

dpm sandbox &
trap 'kill 0' EXIT INT TERM

for _ in $(seq 1 60); do
  curl -fs "$JSON_API/readyz" >/dev/null 2>&1 && break
  sleep 2
done
curl -fs "$JSON_API/readyz" >/dev/null || { echo "ledger-start: sandbox did not become ready" >&2; exit 1; }

curl -fsS -X POST "$JSON_API/v2/packages" \
  -H 'Content-Type: application/octet-stream' \
  --data-binary @test-token-v2/.daml/dist/splice-test-token-v2-1.0.1.dar >/dev/null

dpm script --dar setup/.daml/dist/otc-desk-setup-1.0.0.dar --script-name Setup:setup \
  --ledger-host localhost --ledger-port 6865 --upload-dar true

echo "ledger-start: ready — JSON API $JSON_API, gRPC localhost:6865"
wait

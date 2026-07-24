#!/usr/bin/env bash
# Enforce total statement coverage ≥ threshold for hand-written packages.
# Excludes: generated proto, cmd entrypoints, CLI UI, app wiring, test helpers, scripts.
set -euo pipefail

THRESHOLD="${COVER_THRESHOLD:-70}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

mapfile -t PACKAGES < <(go list ./internal/... ./pkg/... | grep -vE '/internal/proto$|/internal/testutil$|/internal/client/cli$|/internal/server/app$|/internal/server/storage/postgres$')
OUT="$ROOT/coverage.out"
FUNC="$(mktemp)"
trap 'rm -f "$FUNC"' EXIT

echo "Packages under coverage gate:"
printf '  %s\n' "${PACKAGES[@]}"

go test "${PACKAGES[@]}" -covermode=atomic -coverprofile="$OUT" -count=1
go tool cover -func="$OUT" | tee "$FUNC"
go run ./scripts/checkcoverage -threshold "$THRESHOLD" -func "$FUNC"

# Postgres integration (skipped without DATABASE_URL; required in CI).
go test ./internal/server/storage/postgres -count=1

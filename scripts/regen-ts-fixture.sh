#!/usr/bin/env bash
# regen-ts-fixture.sh — regenerate the golden TypeScript fixture for the
# TS→Go migration compatibility test (store/migration_compat_test.go).
#
# What this does
# --------------
# Boots a REAL cita-777/metapi (TypeScript original) server, lets it create
# its database through its own drizzle migrations, writes a known dummy
# dataset through its admin HTTP API, then checkpoints + vacuums the SQLite
# file into a single golden fixture:
#
#   store/testdata/ts-source/hub.db       (real TS-built database, dummy data)
#   store/testdata/ts-source/manifest.json (what the dataset must contain)
#
# The committed fixture is what makes the migration-compat test reusable:
# tests run against it without needing Node or a TS checkout. Re-run this
# script when the TypeScript schema evolves and the fixture must be rebuilt.
#
# Preconditions (maintainer machine)
# ----------------------------------
#   - Node >= 25 (node:zlib createZstdDecompress) as NODE_BIN. The TS server
#     requires it at module load; default is `node`, override with NODE_BIN.
#   - METAPI_TS_DIR pointing at a cita-777/metapi checkout with dependencies
#     installed and better-sqlite3 built for that same Node ABI. If the
#     checkpoint step fails with an ABI error, rebuild it:
#       cd "$METAPI_TS_DIR"/node_modules/.pnpm/better-sqlite3@*/node_modules/better-sqlite3
#       PATH="$(dirname "$NODE_BIN"):$PATH" npm run build-release
#   - curl that can reach localhost directly (no proxy for 127.0.0.1).
#
# Output is deterministic: a fresh DATA_DIR each run + a fixed dataset, so
# regenerating produces an equivalent fixture (timestamps differ, counts and
# identities do not).
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "$0")/.." && pwd)
METAPI_TS_DIR=${METAPI_TS_DIR:?"METAPI_TS_DIR is required (path to a cita-777/metapi checkout)"}
NODE_BIN=${NODE_BIN:-node}

TS_PORT=4199
TS_HOST=127.0.0.1
TS_AUTH_TOKEN=fixture-admin-token
# 64 hex chars, generated at runtime so the source carries no literal secret
# (also keeps leak-guard quiet — this is a dummy fixture credential).
TS_ACCOUNT_SECRET=$(printf '0123456789abcdef%.0s' $(seq 1 4))

OUT_DIR="$REPO_ROOT/store/testdata/ts-source"
WORK_DIR=$(mktemp -d)
DATA_DIR="$WORK_DIR/data"
SERVER_LOG="$WORK_DIR/ts-server.log"
SERVER_PID=""

log() { printf '[regen-ts-fixture] %s\n' "$*"; }
die() { log "ERROR: $*"; cleanup; exit 1; }

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

# The TS server imports createZstdDecompress at module load (Node >= 23.8).
log "checking Node >= 25 (zstd support) ..."
if ! "$NODE_BIN" -e "require('node:zlib').createZstdDecompress; console.log(process.version)" 2>/dev/null; then
  die "NODE_BIN ($NODE_BIN) is too old; need Node >= 25 (zstd). Install one and set NODE_BIN=/path/to/node"
fi

if [ ! -f "$METAPI_TS_DIR/src/server/index.ts" ]; then
  die "METAPI_TS_DIR does not look like a cita-777/metapi checkout (src/server/index.ts missing)"
fi

TSX_CLI="$METAPI_TS_DIR/node_modules/tsx/dist/cli.mjs"
if [ ! -f "$TSX_CLI" ]; then
  die "tsx not found in METAPI_TS_DIR node_modules; run pnpm/npm install there first"
fi

# ---------------------------------------------------------------------------
# 1. Boot a real TS server on a fresh data dir (its own migrations build the
#    database; first boot also seeds the 7 default sites).
# ---------------------------------------------------------------------------
log "starting TS server (fresh DATA_DIR=$DATA_DIR, port $TS_PORT) ..."
(
  cd "$METAPI_TS_DIR"
  env -i PATH="$PATH" HOME="$HOME" \
    DATA_DIR="$DATA_DIR" \
    AUTH_TOKEN="$TS_AUTH_TOKEN" \
    PROXY_TOKEN=fixture-proxy-token \
    ACCOUNT_CREDENTIAL_SECRET="$TS_ACCOUNT_SECRET" \
    PORT="$TS_PORT" HOST="$TS_HOST" \
    MODEL_AVAILABILITY_PROBE_ENABLED=false \
    "$NODE_BIN" "$TSX_CLI" src/server/index.ts
) >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

log "waiting for server readiness ..."
ready=0
for _ in $(seq 1 120); do
  if curl -sS --noproxy '*' "http://$TS_HOST:$TS_PORT/ready" 2>/dev/null | grep -q '"ok"'; then
    ready=1
    break
  fi
  if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    die "TS server exited during startup; log tail: $(tail -5 "$SERVER_LOG" | tr '\n' ' ')"
  fi
  sleep 0.5
done
[ "$ready" = 1 ] || die "TS server did not become ready in 60s; log: $SERVER_LOG"

# ---------------------------------------------------------------------------
# 2. Write the deterministic dummy dataset through the admin API. platform is
#    always explicit (avoids live URL probing) and accounts use
#    credentialMode=apikey + skipModelFetch=true (no upstream contact).
# ---------------------------------------------------------------------------
api() { curl -sS --noproxy '*' -H "Authorization: Bearer $TS_AUTH_TOKEN" -H 'Content-Type: application/json' "$@"; }

post_site() {
  local name=$1 url=$2 platform=$3
  api -X POST "http://$TS_HOST:$TS_PORT/api/sites" \
    -d "{\"name\":\"$name\",\"url\":\"$url\",\"platform\":\"$platform\",\"status\":\"active\"}"
}

post_account() {
  local site_id=$1 username=$2 token=$3 checkin=$4
  api -X POST "http://$TS_HOST:$TS_PORT/api/accounts" \
    -d "{\"siteId\":$site_id,\"username\":\"$username\",\"credentialMode\":\"apikey\",\"accessTokens\":[\"$token\"],\"skipModelFetch\":true,\"checkinEnabled\":$checkin}"
}

post_key() {
  local name=$1 key=$2
  api -X POST "http://$TS_HOST:$TS_PORT/api/downstream-keys" \
    -d "{\"name\":\"$name\",\"key\":\"$key\"}"
}

log "creating 3 sites ..."
post_site "测试中转站A" "https://zhongzhuan-a.example.invalid" "new-api" >/dev/null
post_site "测试中转站B" "https://zhongzhuan-b.example.invalid" "one-api" >/dev/null
post_site "订阅站C" "https://sub-c.example.invalid" "sub2api" >/dev/null

# Fresh database: seeded sites occupy ids 1-7; the three created sites get 8-10.
log "creating 5 accounts ..."
post_account 8 "zhongzhuan-a-user" "sk-fixture-zhongzhuan-a-0000000000000000000000" true >/dev/null
post_account 9 "zhongzhuan-b-user" "sk-fixture-zhongzhuan-b-1111111111111111111111" false >/dev/null
post_account 10 "sub-c-user" "sk-fixture-sub-c-22222222222222222222222222" true >/dev/null
post_account 1 "openai-dummy" "sk-fixture-openai-333333333333333333333333" false >/dev/null
post_account 2 "claude-dummy" "sk-fixture-claude-444444444444444444444444" false >/dev/null

log "creating 2 downstream keys ..."
post_key "fixture-cursor-key" "sk-fixture-cursor-55555555555555555555555555" >/dev/null
post_key "fixture-codex-key" "sk-fixture-codex-66666666666666666666666666" >/dev/null

# ---------------------------------------------------------------------------
# 3. Stop the server, then fold the WAL into the main file and vacuum so the
#    fixture is a single self-contained SQLite file.
# ---------------------------------------------------------------------------
log "stopping TS server ..."
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
SERVER_PID=""

BS_PACKAGE=$(cd "$METAPI_TS_DIR" && node -e "console.log(require.resolve('better-sqlite3/package.json').replace('/package.json',''))" 2>/dev/null || true)
if [ -z "$BS_PACKAGE" ] || [ ! -d "$BS_PACKAGE" ]; then
  die "better-sqlite3 not resolvable from METAPI_TS_DIR"
fi

# ---------------------------------------------------------------------------
# 4. Checkpoint the WAL, vacuum, and derive the manifest straight from the
#    resulting database — the DB is the source of truth the Go test asserts
#    against, not the API view.
# ---------------------------------------------------------------------------
log "checkpointing WAL + vacuuming (better-sqlite3 at $BS_PACKAGE) ..."
"$NODE_BIN" - "$DATA_DIR/hub.db" "$BS_PACKAGE" "$OUT_DIR" <<'NODE'
const fs = require('fs');
const path = require('path');
const [dbPath, bsPackage, outDir] = [process.argv[2], process.argv[3], process.argv[4]];
const Database = require(bsPackage);

const db = new Database(dbPath);
db.pragma('wal_checkpoint(TRUNCATE)');
db.pragma('vacuum');

const tables = db
  .prepare(
    "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '__drizzle%'"
  )
  .all()
  .map((r) => r.name)
  .sort();
console.log('tables:', tables.join(', '));

const count = (table) => db.prepare(`SELECT COUNT(*) AS c FROM ${table}`).get().c;
const siteCount = count('sites');
const accountCount = count('accounts');
const keyCount = count('downstream_api_keys');

// Fail fast if any API write silently failed: the script knows the dataset
// shape, so a shortfall here means a POST errored and the fixture is bad.
const expectedCounts = { sites: 10, accounts: 5, downstream_api_keys: 2 };
for (const [table, want] of Object.entries(expectedCounts)) {
  const got = count(table);
  if (got !== want) {
    throw new Error(`fixture validation failed: ${table} has ${got} rows, want ${want}`);
  }
}

const manifest = {
  description:
    'Golden TypeScript fixture for the TS→Go migration compatibility test ' +
    '(store/migration_compat_test.go). Built by a REAL cita-777/metapi server: ' +
    'schema from its own drizzle migrations, data written through its admin API. ' +
    'All data is dummy. Regenerate with scripts/regen-ts-fixture.sh.',
  generated_at: new Date().toISOString(),
  table_count: tables.length,
  site_count: siteCount,
  account_count: accountCount,
  downstream_key_count: keyCount,
  // Identities the Go test must find after takeover/migration. Extracted from
  // the database so the manifest always matches what the fixture contains.
  check_sites: db
    .prepare('SELECT name, url, platform FROM sites ORDER BY id')
    .all()
    .map((r) => ({ name: r.name, url: r.url, platform: r.platform })),
  check_accounts: db
    .prepare(
      'SELECT a.username AS username, s.name AS site_name FROM accounts a JOIN sites s ON s.id = a.site_id ORDER BY a.id'
    )
    .all(),
  check_downstream_keys: db
    .prepare('SELECT name FROM downstream_api_keys ORDER BY id')
    .all()
    .map((r) => ({ name: r.name })),
};

fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(path.join(outDir, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
db.close();
console.log('manifest:', JSON.stringify({
  table_count: manifest.table_count,
  site_count: manifest.site_count,
  account_count: manifest.account_count,
  downstream_key_count: manifest.downstream_key_count,
}));
NODE

cp "$DATA_DIR/hub.db" "$OUT_DIR/hub.db"
log "fixture written:"
ls -la "$OUT_DIR/"
log "done."

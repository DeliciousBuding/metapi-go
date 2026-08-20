# TS→Go migration compatibility fixture

This directory holds the **golden TypeScript fixture** used by
[`migration_compat_test.go`](../../migration_compat_test.go): a real
`cita-777/metapi` (TypeScript original) database.

- `hub.db` — schema built by the TS server's own drizzle migrations; data
  written through its admin HTTP API. **All data is dummy.**
- `manifest.json` — what the fixture contains (table counts + identity
  spot-checks), extracted straight from the database at build time.

The fixture is committed so the migration-compat tests are reusable without
Node or a TS checkout: they run in every CI job, with the PostgreSQL target
exercised by the `test-pg` job (which already provides a PG server and
`PG_TEST_DSN`).

## Regenerating the fixture

When the TypeScript schema evolves, rebuild the fixture from a real TS server:

```bash
METAPI_TS_DIR=/path/to/cita-777/metapi NODE_BIN=/path/to/node25 \
  bash scripts/regen-ts-fixture.sh
```

Requirements: Node ≥ 25 (the TS server imports `createZstdDecompress`), a
`cita-777/metapi` checkout with dependencies installed and `better-sqlite3`
built for that Node ABI. The script boots a fresh TS server, writes the
deterministic dummy dataset through its admin API, then checkpoints + vacuums
the WAL into a single file. See the script header for details.

## Running the tests

```bash
# SQLite paths (takeover + migrate --verify) — always run
go test ./store/ -run 'TestTSTakeover|TestTSMigrateVerify' -count=1

# PostgreSQL target — runs when PG_TEST_DSN is set, skips otherwise
PG_TEST_DSN='postgres://USER:PASS@HOST:5432/metapi_test?sslmode=disable' \
  go test ./store/ -run 'TestTSMigrateVerifyPG' -count=1
```

The PG test uses a dedicated database (`<dsn-dbname>_ts_compat`) so its
`Overwrite=true` never truncates the shared integration database. The
`PG_TEST_DSN` user needs `CREATEDB` (CI's postgres superuser has it), or the
compat database must be pre-created.

### Local PostgreSQL (one-time setup)

No Docker needed — a dedicated cluster keeps the machine's other PG
untouched:

```bash
# 1. init + start a private cluster (PG refuses to run as root; use its user)
runuser -u postgres -- /usr/lib/postgresql/16/bin/initdb \
  -D /var/lib/postgresql/16/metapi-test --auth=scram-sha-256 --pwfile=/tmp/pw
printf "listen_addresses = '127.0.0.1'\nport = 55432\n" \
  >> /var/lib/postgresql/16/metapi-test/postgresql.conf
runuser -u postgres -- /usr/lib/postgresql/16/bin/pg_ctl \
  -D /var/lib/postgresql/16/metapi-test start

# 2. role + database
psql -h 127.0.0.1 -p 55432 -U postgres \
  -c "CREATE ROLE metapi_test LOGIN PASSWORD 'test' CREATEDB;" \
  -c "CREATE DATABASE metapi_test OWNER metapi_test;"

# 3. run the PG test (the role password set in step 2)
PG_TEST_DSN='postgres://metapi_test:<password>@127.0.0.1:55432/metapi_test?sslmode=disable' \
  go test ./store/ -run 'TestTSMigrateVerifyPG' -count=1
```

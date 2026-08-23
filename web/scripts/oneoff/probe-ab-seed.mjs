#!/usr/bin/env bun
// R2-2 lane Domain-B seeder: injects extreme rows (timestamps / amounts /
// CSV-injection payloads / long+unicode strings) into a THROWAWAY sqlite DB.
// Run with the server STOPPED (avoids write locks): bun scripts/probe-ab-seed.mjs <db>
import { Database } from 'bun:sqlite'

const dbPath = process.argv[2]
if (!dbPath) {
  console.error('usage: bun scripts/probe-ab-seed.mjs <path-to-hub.db>')
  process.exit(2)
}

const db = new Database(dbPath)
db.exec('BEGIN')

function insertSite(name, url, platform) {
  const now = '2026-08-22 12:00:00'
  db.query(
    `INSERT INTO sites (name, url, platform, status, is_pinned, sort_order, global_weight, created_at, updated_at)
     VALUES (?, ?, ?, 'enabled', 0, 0, 1.0, ?, ?)`
  ).run(name, url, platform, now, now)
  return Number(db.query('SELECT last_insert_rowid() AS id').get().id)
}

function insertAccount(siteId, username, balance, extra = {}) {
  const now = '2026-08-22 12:00:00'
  db.query(
    `INSERT INTO accounts (site_id, username, access_token, balance, status, is_pinned, sort_order, checkin_enabled, last_checkin_at, created_at, updated_at)
     VALUES (?, ?, ?, ?, 'active', 0, 0, 1, ?, ?, ?)`
  ).run(
    siteId,
    username,
    `seed-token-${username.slice(0, 8)}`,
    balance,
    extra.lastCheckinAt ?? null,
    now,
    now
  )
  return Number(db.query('SELECT last_insert_rowid() AS id').get().id)
}

function insertProxyLog(fields) {
  db.query(
    `INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, http_status, is_stream, latency_ms, prompt_tokens, completion_tokens, total_tokens, estimated_cost, error_message, retry_count, created_at)
     VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, 0, ?)`
  ).run(
    fields.accountId ?? null,
    fields.modelRequested,
    fields.modelActual ?? fields.modelRequested,
    fields.status ?? 'success',
    fields.httpStatus ?? 200,
    fields.latencyMs ?? 120,
    fields.promptTokens ?? 10,
    fields.completionTokens ?? 20,
    fields.totalTokens ?? 30,
    fields.estimatedCost ?? null,
    fields.errorMessage ?? null,
    fields.createdAt ?? null
  )
}

function insertEvent(type, level, title, message, createdAt) {
  db.query(
    `INSERT INTO events (type, title, message, level, read, created_at) VALUES (?, ?, ?, ?, 0, ?)`
  ).run(type, title, message, level, createdAt)
}

function insertCheckinLog(accountId, status, createdAt, failureReason) {
  db.query(
    `INSERT INTO checkin_logs (account_id, status, message, failure_reason, created_at) VALUES (?, ?, '', ?, ?)`
  ).run(accountId, status, failureReason ?? null, createdAt)
}

// ---- sites: unicode / emoji / long / CSV-hostile names ----
const siteUnicode = insertSite(
  '测试站点🚀にほんご',
  'https://unicode.example.com',
  'new-api'
)
const siteLongName = insertSite(
  'L'.repeat(300),
  'https://long.example.com',
  'new-api'
)
const siteCsvName = insertSite(
  '=@SUM(site)',
  'https://csvsite.example.com',
  'new-api'
)

// ---- accounts: extreme balances + hostile usernames ----
const accountNullBalance = insertAccount(siteUnicode, '=1+1', null)
const accountHugeBalance = insertAccount(
  siteUnicode,
  '巨额账号💰',
  99999999999.99
)
const accountNegativeBalance = insertAccount(
  siteLongName,
  'negative-bal',
  -123.456789
)
const accountTinyBalance = insertAccount(siteLongName, 'tiny-bal', 0.0000001)
const accountLongName = insertAccount(
  siteCsvName,
  '账'.repeat(120) + '🚀'.repeat(40),
  42.5,
  { lastCheckinAt: '3026-01-01 00:00:00' }
)
const accountCmdName = insertAccount(siteCsvName, "-cmd|'/C calc'!A0", 7.25, {
  lastCheckinAt: 'not-a-date',
})

// ---- proxy_logs: timestamp + amount + CSV-injection matrix ----
const timestampVariants = [
  '0',
  '',
  null,
  '3026-08-22 12:00:00',
  '1950-01-01 00:00:00',
  'not-a-date-xyz',
  '2026-08-22T08:00:00+08:00',
  '1755849600',
  '2026-08-22 12:00:00',
]
timestampVariants.forEach((createdAt, index) => {
  insertProxyLog({
    accountId: accountNullBalance,
    modelRequested: `ts-variant-${index}`,
    createdAt,
    estimatedCost: 0.05,
  })
})

insertProxyLog({
  accountId: accountHugeBalance,
  modelRequested: '=1+1',
  estimatedCost: 1e18,
  totalTokens: 123456789012,
})
insertProxyLog({
  accountId: accountHugeBalance,
  modelRequested: "=cmd|'/C calc'!A0",
  estimatedCost: -5.5,
  status: 'failed',
  httpStatus: 500,
  errorMessage: 'boom, "quoted"\nand multiline',
})
insertProxyLog({
  accountId: accountNegativeBalance,
  modelRequested: '+1+1',
  estimatedCost: 0.123456789,
})
insertProxyLog({
  accountId: accountNegativeBalance,
  modelRequested: '@SUM(A1:A2)',
  estimatedCost: 99999999,
})
insertProxyLog({
  accountId: accountCmdName,
  modelRequested: '-1-1',
  estimatedCost: null,
  totalTokens: null,
})
insertProxyLog({
  accountId: accountCmdName,
  modelRequested: '模型'.repeat(80) + '🚀',
  estimatedCost: 0.0001,
})

// ---- events: CSV-injection + formatting payloads for program-logs export ----
insertEvent(
  'system',
  'error',
  '=1+1',
  '=HYPERLINK("http://evil.example")',
  '2026-08-22 12:00:00'
)
insertEvent('system', 'warn', '-cmd', '+calc', '0')
insertEvent(
  'system',
  'info',
  '@SUM(A1:A2)',
  'line1\nline2 "quoted", comma',
  '3026-01-01 00:00:00'
)
insertEvent(
  'checkin',
  'info',
  '正常事件标题',
  '正常消息内容',
  '2026-08-22 11:00:00'
)
insertEvent('system', 'error', 'tab\tafter', 'cr\rlf', '1950-01-01 00:00:00')

// ---- checkin_logs: extreme timestamps ----
insertCheckinLog(accountNullBalance, 'success', '0')
insertCheckinLog(accountNullBalance, 'failed', 'not-a-date', '=fail(reason)')
insertCheckinLog(accountHugeBalance, 'success', '3026-08-22 12:00:00')
insertCheckinLog(accountCmdName, 'failed', '', '-timeout')

db.exec('COMMIT')

const counts = {
  sites: db.query('SELECT COUNT(*) AS n FROM sites').get().n,
  accounts: db.query('SELECT COUNT(*) AS n FROM accounts').get().n,
  proxyLogs: db.query('SELECT COUNT(*) AS n FROM proxy_logs').get().n,
  events: db.query('SELECT COUNT(*) AS n FROM events').get().n,
  checkinLogs: db.query('SELECT COUNT(*) AS n FROM checkin_logs').get().n,
}
console.log('seeded:', JSON.stringify(counts))
db.close()

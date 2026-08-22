#!/usr/bin/env bun
// R2-2 lane: reseed ONLY proxy_logs (previous inline attempt lost bindings).
// Run with the server STOPPED.
import { Database } from 'bun:sqlite'

const dbPath = process.argv[2] ?? '../.tmp-ab/data/hub.db'
const db = new Database(dbPath)

db.exec('BEGIN')
db.exec('DELETE FROM proxy_logs')

const insert = db.query(
  `INSERT INTO proxy_logs (account_id, model_requested, model_actual, status, http_status, is_stream, latency_ms, prompt_tokens, completion_tokens, total_tokens, estimated_cost, error_message, retry_count, created_at)
   VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, 0, ?)`
)

function addLog(fields) {
  insert.run(
    fields.accountId,
    fields.modelRequested,
    fields.modelRequested,
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

// account ids from probe-ab-seed.mjs: 1==1+1 / 2=huge / 3=negative / 4=tiny /
// 5=long / 6=cmd
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
  addLog({
    accountId: 1,
    modelRequested: `ts-variant-${index}`,
    estimatedCost: 0.05,
    createdAt,
  })
})

addLog({
  accountId: 2,
  modelRequested: '=1+1',
  estimatedCost: 1e18,
  totalTokens: 123456789012,
  createdAt: '2026-08-22 12:01:00',
})
addLog({
  accountId: 2,
  modelRequested: "=cmd|'/C calc'!A0",
  estimatedCost: -5.5,
  status: 'failed',
  httpStatus: 500,
  errorMessage: 'boom, "quoted"\nand multiline',
  createdAt: '2026-08-22 12:02:00',
})
addLog({
  accountId: 3,
  modelRequested: '+1+1',
  estimatedCost: 0.123456789,
  createdAt: '2026-08-22 12:03:00',
})
addLog({
  accountId: 3,
  modelRequested: '@SUM(A1:A2)',
  estimatedCost: 99999999,
  createdAt: '2026-08-22 12:04:00',
})
addLog({
  accountId: 6,
  modelRequested: '-1-1',
  estimatedCost: null,
  totalTokens: null,
  createdAt: '2026-08-22 12:05:00',
})
addLog({
  accountId: 6,
  modelRequested: '模型'.repeat(80) + '🚀',
  estimatedCost: 0.0001,
  createdAt: '2026-08-22 12:06:00',
})

db.exec('COMMIT')

const verify = db
  .query(
    'SELECT id, model_requested, estimated_cost, created_at FROM proxy_logs ORDER BY id LIMIT 5'
  )
  .all()
console.log(
  'proxy_logs reseeded:',
  db.query('SELECT COUNT(*) AS n FROM proxy_logs').get().n
)
console.log('verify:', JSON.stringify(verify))
db.close()

import assert from 'node:assert/strict'
import test from 'node:test'

import {
  checkinRunStatus,
  checkinVerdict,
  freshCheckinLog,
  freshUpstreamToken,
  readAccountTokens,
  readCheckinPage,
  readyAccountToken,
  requireTokenChannel,
  resolveAcceptanceAccount,
} from '../acceptance-evidence.mjs'

// Fixtures test the acceptance instrument only. No server, browser, credentials
// or upstream is involved; these results are NOT live acceptance evidence.
const target = {
  accountId: 7,
  username: 'acceptance-user',
  siteName: 'acceptance-site',
  siteUrl: 'https://upstream.example.test',
}
const record = (id = 11, status = 'success') => ({
  checkin_logs: {
    id,
    accountId: target.accountId,
    status,
    message: 'upstream result',
    reward: null,
    createdAt: '2026-09-07T00:00:00Z',
  },
  accounts: { id: target.accountId, username: target.username },
  sites: { name: target.siteName, url: target.siteUrl },
  failureReason: null,
})
const page = (...items) => ({
  items,
  total: items.length,
  page: 1,
  pageSize: 200,
})
const trigger = (status = 'success') => ({
  status: 'completed',
  queued: false,
  results: [
    {
      AccountID: target.accountId,
      Username: target.username,
      Site: target.siteName,
      Result: { Status: status },
    },
  ],
})
const visible = (row = record()) => ({
  id: row.checkin_logs.id,
  account: row.accounts.username,
  site: row.sites.name,
  siteUrl: row.sites.url,
  tableStatus: { success: 'Success', failed: 'Failed', skipped: 'Skipped' }[
    row.checkin_logs.status
  ],
  detailStatus: { success: 'Success', failed: 'Failed', skipped: 'Skipped' }[
    row.checkin_logs.status
  ],
})
function verdict(
  after,
  { ui = visible(), run = trigger(), expected = '0' } = {}
) {
  // The old UI row is deliberately still visible unless the fixture changes it.
  const fresh = freshCheckinLog(readCheckinPage(after, target), 10)
  return checkinVerdict(fresh, ui, checkinRunStatus(run, target), expected)
}

test('fresh success for the exact account passes only with matching table and detail', () => {
  assert.equal(verdict(page(record()), { expected: '1' }), 'PASS')
})

test('historical success row and a completed trigger cannot produce PASS', () => {
  const historical = record(10)
  assert.throws(
    () => verdict(page(historical), { ui: visible(historical) }),
    /no fresh check-in log/
  )
})

test('no new row is FAIL, not success or an unsupported SKIP', () => {
  assert.throws(() => verdict(page()), /no fresh check-in log/)
})

test('a fresh log for another account with the same username is FAIL', () => {
  const wrong = record()
  wrong.checkin_logs.accountId = 8
  wrong.accounts.id = 8
  assert.throws(() => verdict(page(wrong)), /different account or site/)
})

test('wrong joined account, username, site name or URL cannot flatter the target', () => {
  for (const change of [
    (row) => {
      row.accounts.id = 8
    },
    (row) => {
      row.accounts.username += '-other'
    },
    (row) => {
      row.sites.name += '-other'
    },
    (row) => {
      row.sites.url = 'https://other.example.test'
    },
  ]) {
    const wrong = record()
    change(wrong)
    assert.throws(() => verdict(page(wrong)), /different account or site/)
  }
})

test('fresh failed log stays FAIL even when its message claims success', () => {
  const failed = record(11, 'failed')
  failed.checkin_logs.message = 'success; already checked in'
  assert.throws(
    () =>
      verdict(page(failed), { ui: visible(failed), run: trigger('failed') }),
    /failed/
  )
})

test('an explicit fresh skipped log is SKIP by default, never PASS', () => {
  const skipped = record(11, 'skipped')
  assert.equal(
    verdict(page(skipped), { ui: visible(skipped), run: trigger('skipped') }),
    'SKIP'
  )
})

test('required-checkin mode makes even an explicit skipped log FAIL', () => {
  const skipped = record(11, 'skipped')
  assert.throws(
    () =>
      verdict(page(skipped), {
        ui: visible(skipped),
        run: trigger('skipped'),
        expected: '1',
      }),
    /skipped.*successful check-in was required/
  )
})

test('unknown or missing log status is malformed, never a wording-based success/skip', () => {
  for (const status of [
    undefined,
    null,
    '',
    'completed',
    'unsupported',
    'SUCCESS',
  ]) {
    const malformed = record(11, status)
    malformed.checkin_logs.status = status
    malformed.checkin_logs.message = 'success / unsupported'
    assert.throws(() => verdict(page(malformed)), /malformed check-in log/)
  }
})

test('malformed envelope, ID, time or nested log cannot pass', () => {
  for (const malformed of [
    null,
    [],
    {},
    { items: [] },
    { ...page(), total: -1 },
    { ...page(), pageSize: 0 },
    { ...page(record()), items: [null] },
  ]) {
    assert.throws(() => verdict(malformed), /malformed/)
  }
  for (const change of [
    (row) => {
      row.checkin_logs.id = '11'
    },
    (row) => {
      row.checkin_logs.id = 0
    },
    (row) => {
      row.checkin_logs.createdAt = 'not-a-time'
    },
    (row) => {
      delete row.checkin_logs.message
    },
  ]) {
    const malformed = record()
    change(malformed)
    assert.throws(() => verdict(page(malformed)), /malformed/)
  }
})

test('multiple new outcomes fail closed instead of selecting a flattering success', () => {
  assert.throws(
    () => verdict(page(record(12), record(11, 'failed'))),
    /ambiguous new/
  )
})

test('the triggered round must name the exact account and a normal status', () => {
  for (const change of [
    (run) => {
      run.results = []
    },
    (run) => {
      run.results[0].AccountID = 8
    },
    (run) => {
      run.results[0].Site = 'other-site'
    },
    (run) => {
      run.results[0].Result.Status = 'unsupported'
    },
    (run) => {
      run.results.push(run.results[0])
    },
  ]) {
    const run = trigger()
    change(run)
    assert.throws(() => verdict(page(record()), { run }), /unique valid result/)
  }
  assert.throws(
    () => verdict(page(record()), { run: { status: 'queued' } }),
    /malformed/
  )
  assert.throws(
    () => verdict(page(record()), { run: trigger('skipped') }),
    /does not match/
  )
})

test('API success cannot hide missing, historical, wrong-account or wrong-status UI evidence', () => {
  for (const ui of [
    null,
    { ...visible(), id: 10 },
    { ...visible(), account: 'other-user' },
    { ...visible(), site: 'other-site' },
    { ...visible(), siteUrl: 'https://other.example.test' },
    { ...visible(), tableStatus: 'Failed' },
    { ...visible(), detailStatus: 'Failed' },
  ]) {
    assert.throws(() => verdict(page(record()), { ui }), /UI does not show/)
  }
})

test('a skipped API response also requires a real matching UI outcome', () => {
  const skipped = record(11, 'skipped')
  assert.throws(
    () => verdict(page(skipped), { ui: visible(), run: trigger('skipped') }),
    /UI does not show/
  )
})

test('invalid expectation values cannot silently opt out of check-in', () => {
  assert.throws(
    () => verdict(page(record()), { expected: 'true' }),
    /ACCEPT_EXPECT_CHECKIN/
  )
})

test('account resolution uses both site ID and exact configured site name/URL', () => {
  const snapshot = {
    sites: [
      { id: 2, name: 'other-site', url: 'https://other.example.test' },
      { id: 3, name: target.siteName, url: `${target.siteUrl}/` },
    ],
    accounts: [
      { id: 6, siteId: 2, username: target.username },
      { id: target.accountId, siteId: 3, username: target.username },
    ],
  }
  assert.equal(
    resolveAcceptanceAccount(snapshot, target).accountId,
    target.accountId
  )
  assert.throws(
    () =>
      resolveAcceptanceAccount(snapshot, {
        ...target,
        siteUrl: 'https://wrong.example.test',
      }),
    /exactly one acceptance site/
  )
  snapshot.accounts.push({ ...snapshot.accounts[1], id: 9 })
  assert.throws(
    () => resolveAcceptanceAccount(snapshot, target),
    /exactly one acceptance account/
  )
})

const token = (id = 22) => ({
  id,
  accountId: target.accountId,
  name: 'acceptance-upstream-token',
  valueStatus: 'ready',
  enabled: true,
  isDefault: true,
})
const created = (value = token()) => ({
  success: true,
  synced: true,
  created: 1,
  token: value,
})

test('new upstream-create response must prove a fresh ready/default token', () => {
  assert.deepEqual(
    freshUpstreamToken(created(), [token(21)], target.accountId, token().name),
    token()
  )
  for (const value of [
    token(21),
    { ...token(), valueStatus: 'masked_pending' },
    { ...token(), enabled: false },
    { ...token(), isDefault: false },
    { ...token(), name: 'old-token' },
  ]) {
    assert.throws(
      () =>
        freshUpstreamToken(
          created(value),
          [token(21)],
          target.accountId,
          token().name
        ),
      /fresh ready\/default/
    )
  }
  assert.throws(
    () =>
      freshUpstreamToken(
        created({ ...token(), accountId: 8 }),
        [],
        target.accountId,
        token().name
      ),
    /wrong-account/
  )
})

test('HTTP-success-like envelopes without actual upstream creation cannot pass', () => {
  for (const result of [
    null,
    {},
    { ...created(), success: false },
    { ...created(), synced: false },
    { ...created(), created: 0 },
    { ...created(), token: undefined },
  ]) {
    assert.throws(() =>
      freshUpstreamToken(result, [], target.accountId, token().name)
    )
  }
})

test('reuse requires ready AND enabled; masked or disabled tokens cannot unlock route creation', () => {
  assert.equal(readyAccountToken([]), null)
  assert.equal(
    readyAccountToken([{ ...token(), valueStatus: 'masked_pending' }]),
    null
  )
  assert.equal(readyAccountToken([{ ...token(), enabled: false }]), null)
  assert.equal(
    readyAccountToken([{ ...token(21), isDefault: false }, token()]).id,
    22
  )
})

test('token evidence retains metadata only and rejects wrong-account or malformed lists', () => {
  assert.deepEqual(
    readAccountTokens(
      [
        {
          ...token(),
          token: 'fixture-only-do-not-retain',
          tokenMasked: 'fixture-only',
        },
      ],
      target.accountId
    ),
    [token()]
  )
  assert.throws(() => readAccountTokens(null, target.accountId), /malformed/)
  assert.throws(
    () => readAccountTokens([{ ...token(), accountId: 8 }], target.accountId),
    /wrong-account/
  )
})

test('route must bind the verified token on the verified account, not any non-null tokenId', () => {
  const channel = {
    accountId: target.accountId,
    tokenId: token().id,
    enabled: true,
  }
  requireTokenChannel([channel], target.accountId, token().id)
  requireTokenChannel(
    [{ ...channel, enabled: 1 }],
    target.accountId,
    token().id
  )
  for (const channels of [
    [],
    null,
    [{ ...channel, accountId: 8 }],
    [{ ...channel, tokenId: 21 }],
    [{ ...channel, tokenId: null }],
    [{ ...channel, enabled: false }],
    [{ ...channel, enabled: 0 }],
  ]) {
    assert.throws(
      () => requireTokenChannel(channels, target.accountId, token().id),
      /no enabled channel/
    )
  }
})

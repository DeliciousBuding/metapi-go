// Pure evidence checks shared by the real-UI acceptance journeys and their
// offline negative controls. Never infer an outcome from an upstream message.

const positiveId = (value) => Number.isSafeInteger(value) && value > 0
const checkinStatuses = new Set(['success', 'failed', 'skipped'])
// Display assertions only: the status is always taken from the API enum.
const checkinLabels = {
  success: 'Success',
  failed: 'Failed',
  skipped: 'Skipped',
}

function siteUrl(value) {
  try {
    return new URL(value).href.replace(/\/$/, '')
  } catch {
    throw new Error('malformed acceptance site URL')
  }
}

export function resolveAcceptanceAccount(snapshot, expected) {
  if (!Array.isArray(snapshot?.accounts) || !Array.isArray(snapshot?.sites)) {
    throw new Error('malformed accounts snapshot')
  }
  const sites = snapshot.sites.filter(
    (site) =>
      site?.name === expected.siteName &&
      siteUrl(site.url) === siteUrl(expected.siteUrl)
  )
  if (sites.length !== 1 || !positiveId(sites[0].id)) {
    throw new Error(
      'expected exactly one acceptance site with the configured name and URL'
    )
  }
  const accounts = snapshot.accounts.filter(
    (account) =>
      account?.siteId === sites[0].id && account?.username === expected.username
  )
  if (accounts.length !== 1 || !positiveId(accounts[0].id)) {
    throw new Error(
      'expected exactly one acceptance account on the target site'
    )
  }
  return {
    accountId: accounts[0].id,
    username: accounts[0].username,
    siteName: sites[0].name,
    siteUrl: sites[0].url,
  }
}

export function readCheckinPage(payload, target) {
  if (
    !Array.isArray(payload?.items) ||
    !Number.isSafeInteger(payload.total) ||
    payload.total < payload.items.length ||
    !positiveId(payload.page) ||
    !positiveId(payload.pageSize) ||
    payload.items.length > payload.pageSize
  ) {
    throw new Error('malformed check-in logs page')
  }
  const ids = new Set()
  for (const row of payload.items) {
    const log = row?.checkin_logs
    if (
      !positiveId(log?.id) ||
      !positiveId(log?.accountId) ||
      !checkinStatuses.has(log?.status) ||
      typeof log?.createdAt !== 'string' ||
      !Number.isFinite(Date.parse(log.createdAt)) ||
      (log.message !== null && typeof log.message !== 'string') ||
      (log.reward !== null && typeof log.reward !== 'string') ||
      ids.has(log.id)
    ) {
      throw new Error('malformed check-in log')
    }
    ids.add(log.id)
    if (
      log.accountId !== target.accountId ||
      row.accounts?.id !== target.accountId ||
      row.accounts?.username !== target.username ||
      row.sites?.name !== target.siteName ||
      siteUrl(row.sites?.url) !== siteUrl(target.siteUrl)
    ) {
      throw new Error('check-in log belongs to a different account or site')
    }
  }
  return payload
}

export function freshCheckinLog(page, cursor) {
  if (!Number.isSafeInteger(cursor) || cursor < 0) {
    throw new Error('invalid pre-trigger check-in cursor')
  }
  const fresh = page.items.filter((row) => row.checkin_logs.id > cursor)
  if (fresh.length > 1) {
    throw new Error('ambiguous new check-in logs for the target account')
  }
  return fresh[0] ?? null
}

export function checkinRunStatus(payload, target) {
  if (
    payload?.status !== 'completed' ||
    payload.queued !== false ||
    !Array.isArray(payload.results)
  ) {
    throw new Error('malformed check-in trigger response')
  }
  // CheckinAllResult currently has no JSON tags: these wire fields are Go-cased.
  const results = payload.results.filter(
    (entry) =>
      entry?.AccountID === target.accountId &&
      entry.Username === target.username &&
      entry.Site === target.siteName
  )
  if (
    results.length !== 1 ||
    !checkinStatuses.has(results[0]?.Result?.Status)
  ) {
    throw new Error(
      'check-in trigger has no unique valid result for the target account'
    )
  }
  return results[0].Result.Status
}

export function checkinVerdict(
  row,
  ui,
  runStatus,
  expected = '0',
  expectedReward = ''
) {
  if (expected !== '0' && expected !== '1') {
    throw new Error('ACCEPT_EXPECT_CHECKIN must be 0 or 1')
  }
  if (!row) throw new Error('no fresh check-in log for the target account')
  const log = row.checkin_logs
  if (!checkinStatuses.has(log.status) || log.status !== runStatus) {
    throw new Error(
      'fresh check-in log does not match the UI-triggered run status'
    )
  }
  if (
    ui?.id !== log.id ||
    ui.account !== row.accounts.username ||
    ui.site !== row.sites.name ||
    siteUrl(ui.siteUrl) !== siteUrl(row.sites.url) ||
    ui.tableStatus !== checkinLabels[log.status] ||
    ui.detailStatus !== checkinLabels[log.status]
  ) {
    throw new Error(
      `UI does not show check-in log #${log.id} with its real result`
    )
  }
  if (
    expectedReward &&
    (log.reward !== expectedReward || ui.reward !== expectedReward)
  ) {
    throw new Error(
      'check-in reward does not match the independently configured upstream reward'
    )
  }
  if (log.status === 'success') return 'PASS'
  if (log.status === 'skipped' && expected === '0') return 'SKIP'
  throw new Error(
    `check-in log #${log.id}: ${log.status}; a successful check-in was required`
  )
}

export function readAccountTokens(payload, accountId) {
  if (!Array.isArray(payload)) {
    throw new Error('malformed account tokens response')
  }
  for (const token of payload) {
    if (
      !positiveId(token?.id) ||
      token.accountId !== accountId ||
      typeof token.name !== 'string' ||
      typeof token.enabled !== 'boolean' ||
      typeof token.isDefault !== 'boolean' ||
      !['ready', 'masked_pending'].includes(token.valueStatus)
    ) {
      throw new Error('malformed or wrong-account token metadata')
    }
  }
  // Retain only non-secret evidence. In particular, never read /:id/value.
  return payload.map(
    ({ id, accountId, name, valueStatus, enabled, isDefault }) => ({
      id,
      accountId,
      name,
      valueStatus,
      enabled,
      isDefault,
    })
  )
}

export function readyAccountToken(tokens) {
  const ready = tokens.filter(
    (token) => token.valueStatus === 'ready' && token.enabled === true
  )
  return ready.find((token) => token.isDefault) ?? ready[0] ?? null
}

export function freshUpstreamToken(payload, before, accountId, name) {
  if (
    payload?.success !== true ||
    payload.synced !== true ||
    !Number.isSafeInteger(payload.created) ||
    payload.created < 1
  ) {
    throw new Error('UI submission did not create and sync an upstream token')
  }
  const [token] = readAccountTokens([payload.token], accountId)
  if (
    !readyAccountToken([token]) ||
    !token.isDefault ||
    token.name !== name ||
    before.some((entry) => entry.id === token.id)
  ) {
    throw new Error(
      'UI submission did not return a fresh ready/default upstream token'
    )
  }
  return token
}

// The channel endpoint exposes DB booleans: true on PG, 1 on SQLite.
export function requireTokenChannel(
  channels,
  accountId,
  tokenId,
  defaultToken = null
) {
  const isVerifiedDefault =
    defaultToken?.id === tokenId &&
    defaultToken.accountId === accountId &&
    defaultToken.isDefault === true &&
    readyAccountToken([defaultToken]) !== null
  if (
    !positiveId(accountId) ||
    !positiveId(tokenId) ||
    !Array.isArray(channels) ||
    !channels.some(
      (channel) =>
        channel?.accountId === accountId &&
        (channel.enabled === true || channel.enabled === 1) &&
        (channel.tokenId === tokenId ||
          (channel.tokenId === null &&
            isVerifiedDefault &&
            typeof channel.account?.apiTokenMasked === 'string' &&
            channel.account.apiTokenMasked.trim() !== ''))
    )
  ) {
    throw new Error(
      'route has no enabled channel using the verified account token or its account default'
    )
  }
}

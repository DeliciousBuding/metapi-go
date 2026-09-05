type MissingTokenModelAccount = {
  accountId: number
  username: string | null
  siteId: number
  siteName: string
  missingGroups?: string[]
  requiredGroups?: string[]
  availableGroups?: string[]
  groupCoverageUncertain?: boolean
}

export type MissingTokenModelsByName = Record<
  string,
  MissingTokenModelAccount[]
>

export function normalizeMissingTokenModels(
  withoutTokenByModel: MissingTokenModelsByName
): MissingTokenModelsByName {
  const normalized: MissingTokenModelsByName = {}
  for (const modelName of Object.keys(withoutTokenByModel || {})) {
    const normalizedModelName = String(modelName || '').trim()
    if (!normalizedModelName) continue
    const accountMap = new Map<number, MissingTokenModelAccount>()
    for (const account of withoutTokenByModel[modelName] || []) {
      if (!account || !Number.isFinite(account.accountId)) continue
      const accountName = (account.username || '').trim()
      const siteName = String(account.siteName || '').trim()
      const normalizeLabels = (labels: unknown): string[] =>
        Array.isArray(labels)
          ? [
              ...new Set(
                labels
                  .map((label) => String(label || '').trim())
                  .filter(Boolean)
              ),
            ].sort((a, b) =>
              a.localeCompare(b, undefined, { sensitivity: 'base' })
            )
          : []
      accountMap.set(account.accountId, {
        accountId: account.accountId,
        username: accountName || null,
        siteId: account.siteId,
        siteName,
        ...(normalizeLabels(account.missingGroups).length > 0
          ? { missingGroups: normalizeLabels(account.missingGroups) }
          : {}),
        ...(normalizeLabels(account.requiredGroups).length > 0
          ? { requiredGroups: normalizeLabels(account.requiredGroups) }
          : {}),
        ...(normalizeLabels(account.availableGroups).length > 0
          ? { availableGroups: normalizeLabels(account.availableGroups) }
          : {}),
        ...(account.groupCoverageUncertain === true
          ? { groupCoverageUncertain: true }
          : {}),
      })
    }
    if (accountMap.size > 0) {
      normalized[normalizedModelName] = [...accountMap.values()].sort(
        (a, b) => a.accountId - b.accountId
      )
    }
  }
  return normalized
}

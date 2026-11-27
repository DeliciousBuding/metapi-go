/** POST /api/models/probe — queue a background availability probe pass. */
export type TriggerModelProbeResponse = {
  success: boolean
  queued: boolean
  reused: boolean
  jobId: string
  status: string
  message?: string
}

// G1: batch model verification + history.
export type ModelVerifyItem = {
  id?: number | null
  batchId?: string
  model: string
  channelId?: number | null
  accountId?: number | null
  siteId?: number | null
  siteName?: string
  status: 'success' | 'failure' | 'inconclusive' | 'skipped'
  latencyMs?: number | null
  httpStatus?: number | null
  errorText?: string | null
  healthApplied?: boolean
  createdAt?: string
}

export type VerifyBatchResponse = {
  success: boolean
  batchId: string
  probed: number
  summary: {
    success: number
    failure: number
    inconclusive: number
    skipped: number
  }
  items: ModelVerifyItem[]
  note?: string
}

export type VerifyHistoryResponse = { items: ModelVerifyItem[] }

// K1a: model name redirects.
export type ModelRedirect = {
  id: number
  accountId: number
  username?: string
  siteName?: string
  canonical: string
  actual: string
  source: 'sync' | 'manual'
  lastSeenAt?: string | null
  createdAt: string
  updatedAt: string
}

export type ModelRedirectsResponse = { items: ModelRedirect[] }

// Disabled-model entries that a redirect mapping can repair (K1a fix
// candidates). Only surfaced via the candidates field of the model-redirects
// apply endpoint (Settings > Models > Redirects preview/apply) since the
// standalone fix-candidates page was folded in.
export type RedirectApplyResponse = {
  success: boolean
  dryRun: boolean
  candidates?: Array<{
    siteId: number
    siteName: string
    accountId: number
    modelName: string
    canonical: string
    actual: string
  }>
  count: number
  removed?: number
}

/**
 * Model-catalog data source registry.
 */
export type CatalogSource = {
  id: number
  name: string
  url: string
  enabled: boolean
  type: 'official' | 'custom'
  sortOrder: number
  lastSuccessAt: string | null
  lastError: string | null
  lastCount: number
  lastAttemptAt: string | null
}

export type CatalogSourceInput = {
  name?: string
  url?: string
  enabled?: boolean
  type?: 'official' | 'custom'
  sortOrder?: number
}

export type CatalogSyncStatus = {
  /** Older servers returned status only while the runtime was enabled. */
  enabled?: boolean
  autoSync: boolean
  intervalMin: number
  snapshot: {
    source: string
    fetchedAt: string | null
    models: number
  }
  sources: CatalogSource[]
}

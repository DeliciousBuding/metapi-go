// metapi-go/features/import — domain types for the unified import wizard.
//
// The wizard turns a list of pasted URLs into candidate sites (auto-detected
// platform + confidence), lets the operator attach accounts and routing weight,
// then submits one idempotent POST /api/sites/import batch.

type ImportAccountInput = {
  username?: string | null
  accessToken?: string
  apiToken?: string
}

export type ImportSiteItem = {
  name: string
  url: string
  platform?: string
  globalWeight?: number
  maxConcurrency?: number
  accounts?: ImportAccountInput[]
}

export type ImportDuplicateStrategy = 'skip' | 'merge'

export type ImportSitesPayload = {
  items: ImportSiteItem[]
  duplicateStrategy: ImportDuplicateStrategy
}

export type ImportSiteResultStatus =
  | 'imported'
  | 'merged'
  | 'skipped'
  | 'failed'

type ImportSiteItemResult = {
  name: string
  url: string
  status: ImportSiteResultStatus
  reason?: string
  siteId?: number
}

export type ImportSitesResult = {
  imported: number
  skipped: number
  failed: number
  results: ImportSiteItemResult[]
}

/** Platform detection response from POST /api/sites/detect. */
export type SiteDetectResult = {
  url?: string
  canonicalUrl?: string
  platform?: string
  siteType?: string
  confidence?: number
  initializationPresetId?: string | null
}

/** One candidate row held in wizard state across the five steps. */
export type ImportCandidate = {
  id: string
  url: string
  name: string
  platform: string
  confidence: number | null
  detected: boolean
  detecting: boolean
  duplicateStrategy: ImportDuplicateStrategy
  includeAccount: boolean
  username: string
  accessToken: string
  apiToken: string
  weight: number
}

export type ImportStepId = 'source' | 'identify' | 'connect' | 'routes' | 'done'

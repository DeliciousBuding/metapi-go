// metapi-go features/accounts — entity types & runtime schemas.
//
// The Account/AccountToken shapes mirror the enriched rows returned by
// GET /api/accounts (snapshot) and GET /api/account-tokens. api.ts casts the
// JSON body to the contract type without runtime checks, so these Zod schemas
// parse each row defensively before feature code touches it — mirroring the
// keys feature's `apiKeySchema.parse(row.original)` safety pattern.

import { z } from 'zod'

// ---------------------------------------------------------------------------
// Status enums
// ---------------------------------------------------------------------------

const ACCOUNT_STATUS_VALUES = ['active', 'disabled', 'expired'] as const
export type AccountStatus = (typeof ACCOUNT_STATUS_VALUES)[number]

const RUNTIME_HEALTH_STATES = [
  'healthy',
  'unhealthy',
  'degraded',
  'disabled',
  'unknown',
] as const
export type RuntimeHealthState = (typeof RUNTIME_HEALTH_STATES)[number]

const CREDENTIAL_MODES = ['session', 'apikey', 'password'] as const
export type CredentialMode = (typeof CREDENTIAL_MODES)[number]

// ---------------------------------------------------------------------------
// Site (minimal projection carried on each account row)
// ---------------------------------------------------------------------------

const siteSchema = z.object({
  id: z.coerce.number(),
  name: z.string().nullish().default(''),
  url: z.string().nullish().default(''),
  platform: z.string().nullish().default(''),
  status: z.string().nullish().default(''),
})
export type Site = z.infer<typeof siteSchema>

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

export const accountSchema = z.object({
  id: z.coerce.number(),
  siteId: z.coerce.number(),
  username: z.string().nullish().default(null),
  status: z.enum(ACCOUNT_STATUS_VALUES).catch('active'),
  accessToken: z.string().nullish().default(''),
  apiToken: z.string().nullish().default(null),
  accessTokenMasked: z.string().nullish().default(''),
  apiTokenMasked: z.string().nullish().default(''),
  balance: z.coerce.number().nullish().default(0),
  balanceUsed: z.coerce.number().nullish().default(0),
  quota: z.coerce.number().nullish().default(0),
  unitCost: z.coerce.number().nullish().default(null),
  valueScore: z.coerce.number().nullish().default(0),
  isPinned: z.coerce.boolean().nullish().default(false),
  sortOrder: z.coerce.number().nullish().default(0),
  checkinEnabled: z.coerce.boolean().nullish().default(false),
  lastCheckinAt: z.string().nullish().default(null),
  lastBalanceRefresh: z.string().nullish().default(null),
  credentialMode: z.enum(CREDENTIAL_MODES).catch('session'),
  capabilities: z
    .object({
      canCheckin: z.coerce.boolean().nullish().default(false),
      canRefreshBalance: z.coerce.boolean().nullish().default(false),
      proxyOnly: z.coerce.boolean().nullish().default(false),
    })
    .nullish()
    .default({ canCheckin: false, canRefreshBalance: false, proxyOnly: false }),
  runtimeHealth: z
    .object({
      state: z.enum(RUNTIME_HEALTH_STATES).catch('unknown'),
      reason: z.string().nullish().default(''),
    })
    .nullish()
    .default({ state: 'unknown', reason: '' }),
  site: siteSchema.nullish(),
  todayReward: z.coerce.number().nullish().default(0),
  todayRewardStatus: z.string().nullish().default(''),
  todaySpend: z.coerce.number().nullish().default(0),
  todaySpendStatus: z.string().nullish().default(''),
  tags: z
    .union([z.string(), z.array(z.string())])
    .nullish()
    .transform((value) => {
      if (Array.isArray(value)) return value
      if (typeof value === 'string' && value.trim()) {
        try {
          const parsed = JSON.parse(value)
          return Array.isArray(parsed) ? parsed.map(String) : [value]
        } catch {
          return value
            .split(/[,，]/)
            .map((tag) => tag.trim())
            .filter(Boolean)
        }
      }
      return []
    })
    .default([]),
  extraConfig: z.string().nullish().default(null),
  createdAt: z.string().nullish().default(''),
  updatedAt: z.string().nullish().default(''),
})
export type Account = z.infer<typeof accountSchema>

// ---------------------------------------------------------------------------
// AccountToken
// ---------------------------------------------------------------------------

export const accountTokenSchema = z.object({
  id: z.coerce.number(),
  accountId: z.coerce.number().nullish().default(0),
  name: z.string().nullish().default(''),
  token: z.string().nullish().default(''),
  tokenMasked: z.string().nullish().default(''),
  tokenGroup: z.string().nullish().default(null),
  valueStatus: z.string().catch('normal'),
  source: z.string().nullish().default(''),
  enabled: z.coerce.boolean().nullish().default(true),
  isDefault: z.coerce.boolean().nullish().default(false),
  createdAt: z.string().nullish().default(''),
  updatedAt: z.string().nullish().default(''),
})
export type AccountToken = z.infer<typeof accountTokenSchema>

// ---------------------------------------------------------------------------
// Snapshot (GET /api/accounts) — returned unwrapped by api.getAccountsSnapshot
// ---------------------------------------------------------------------------

const accountsSnapshotSchema = z.object({
  generatedAt: z.string().nullish().default(''),
  accounts: z.array(accountSchema).catch([]).default([]),
  sites: z.array(siteSchema).catch([]).default([]),
})
export type AccountsSnapshot = z.infer<typeof accountsSnapshotSchema>

// ---------------------------------------------------------------------------
// Account create/update payload (POST/PUT /api/accounts)
// ---------------------------------------------------------------------------

export interface AccountPayload {
  siteId: number
  credentialMode: CredentialMode
  accessToken?: string
  apiToken?: string
  accessTokens?: string[]
  username?: string
  platformUserId?: number
  status?: AccountStatus
  checkinEnabled?: boolean
  unitCost?: number
  proxyUrl?: string
  refreshToken?: string
  tokenExpiresAt?: number
  skipModelFetch?: boolean
  tags?: string[]
}

// ---------------------------------------------------------------------------
// Account login payload (POST /api/accounts/login) — password-based binding.
// The backend signs in with username+password, then creates the account (or
// updates an existing one for the same site+username) with the upstream
// session token plus autoRelogin config. This is a separate endpoint from
// POST/PUT /api/accounts, so it carries its own payload type.
// ---------------------------------------------------------------------------

export interface LoginAccountPayload {
  siteId: number
  username: string
  password: string
}

// ---------------------------------------------------------------------------
// Dialog state machine (mirrors the keys feature's union-typed provider state)
// ---------------------------------------------------------------------------

export interface AccountRowActions {
  onEdit: (account: Account) => void
  onDelete: (account: Account) => void
  onRefresh: (account: Account) => void
  onViewDetail: (account: Account) => void
  onTogglePin: (account: Account) => void
  onToggleCheckin: (account: Account) => void
  onToggleStatus: (account: Account) => void
}

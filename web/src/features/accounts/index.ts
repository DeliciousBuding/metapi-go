// metapi-go/features/accounts — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
// `AccountsPage` is deliberately not exported here: its route file loads it
// directly, so a feature that only needs the contract below does not pull
// the page into its own chunk.
//
// Type-only re-exports use `export type` (isolatedModules-safe).

// --- account hooks + query keys ---
export { accountQueryKeys, fetchAccountsPage, useAccounts } from './api'

// --- account entity types + runtime schemas ---
export { accountSchema } from './types'
export type { Account, AccountToken, AccountsSnapshot } from './types'

// --- tokens sub-module ---
export { useAllAccountTokens } from './tokens/api'

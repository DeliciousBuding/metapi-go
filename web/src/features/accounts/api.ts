/**
 * Post-create token sync statuses reported truthfully by the backend
 * (handler/admin/account_tokens_sync.go). `synced`/`empty`/`skipped` are
 * informational; `failed` means partial initialization — the account row is
 * persisted but its upstream tokens did not sync.
 */
export type TokenSyncStatus = 'synced' | 'empty' | 'failed' | 'skipped'

export interface VerifyTokenResult {
  tokenType?: string
  modelCount?: number
  models?: unknown[]
  userInfo?: unknown
  balance?: unknown
  apiToken?: string
  apiTokenFound?: boolean
}

export interface CreateAccountResult {
  success?: boolean
  message?: string
  id?: number
  items?: Array<{
    id?: number
    status?: 'created' | 'failed'
  }>
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
}

export interface LoginAccountResult {
  success?: boolean
  message?: string
  account?: { id?: number; username?: string }
  reusedAccount?: boolean
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
}

export interface BatchAccountResult {
  successIds?: number[]
  failedItems?: Array<{ id: number; message?: string }>
}

export {
  accountQueryKeys,
  useAccounts,
  useAccountsPage,
  fetchAccountsPage,
  type AccountsPageData,
} from './account-queries'
export {
  useCreateAccount,
  useDeleteAccount,
  useLoginAccount,
  useUpdateAccount,
  resolveCreatedAccountId,
} from './account-mutations'
export { useVerifyAccountToken } from './account-token-verification'
export {
  useRefreshAccount,
  useBatchUpdateAccounts,
  type BatchAccountAction,
} from './account-maintenance'
export {
  useToggleAccountCheckin,
  useToggleAccountPin,
  useToggleAccountStatus,
} from './account-toggles'

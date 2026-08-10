// metapi-go features/accounts — public barrel.
//
// Consumers should import only from here:
//   import { AccountsPage, useAccounts, type Account } from '@/features/accounts'
//
// `export type` is used for all type-only re-exports (isolatedModules-safe).

// --- page + components ---
export { AccountsPage } from './components/accounts-page'
export { AccountFormDialog } from './components/account-form-dialog'
export { AccountDetailSheet } from './components/account-detail-sheet'
export { showAccountCreatedToast } from './components/account-created-toast'
export { useAccountsColumns } from './components/accounts-columns'

// --- account hooks + query keys ---
export {
  accountQueryKeys,
  useAccounts,
  useCreateAccount,
  useUpdateAccount,
  useRefreshAccount,
  useDeleteAccount,
  useBatchUpdateAccounts,
  useToggleAccountPin,
  useToggleAccountStatus,
  useToggleAccountCheckin,
  selectAccountById,
} from './api'
export type {
  CreateAccountResult,
  BatchAccountAction,
  BatchAccountResult,
} from './api'

// --- account entity types + runtime schemas ---
export type {
  Account,
  AccountToken,
  AccountPayload,
  AccountStatus,
  RuntimeHealthState,
  CredentialMode,
  Site,
  AccountsSnapshot,
  AccountRowActions,
  AccountsDialogType,
} from './types'
export {
  accountSchema,
  accountTokenSchema,
  accountsSnapshotSchema,
  ACCOUNT_STATUS_VALUES,
  RUNTIME_HEALTH_STATES,
  CREDENTIAL_MODES,
  ACCOUNT_TOKEN_VALUE_STATUSES,
} from './types'

// --- account form schema ---
export {
  getAccountFormSchema,
  getAccountFormDefaultValues,
  transformFormToPayload,
  transformAccountToFormValues,
} from './lib/accounts-schema'
export type { AccountFormValues } from './lib/accounts-schema'

// --- tokens sub-module ---
export { TokensPanel } from './tokens/components/tokens-panel'
export {
  accountTokenQueryKeys,
  useAccountTokens,
  useAccountTokenValue,
  useCreateAccountToken,
  useUpdateAccountToken,
  useDeleteAccountToken,
  useSetDefaultAccountToken,
  useSyncAccountTokens,
  useToggleAccountTokenEnabled,
} from './tokens/api'
export type { AccountTokenPayload } from './tokens/lib/tokens-schema'

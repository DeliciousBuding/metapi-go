// metapi-go features/accounts/api — TanStack Query hooks for the accounts
// domain. Establishes the query-key conventions for the rewrite:
//   ['accounts']                — factory root; the invalidation prefix
//   ['accounts','snapshot']     — snapshot list (GET /api/accounts)
//   ['accounts','page',{…}]     — one server-paged table page: what the
//                                 /accounts table actually renders
//   ['accounts','detail',id]    — a single account
//   ['account-tokens', id]      — tokens for an account (see tokens/api.ts)
// `page` and `snapshot` are siblings, so cache writes that must reach the
// table go through `accountQueryKeys.pages()` (or `.all`), never `.snapshot()`.
//
// Mutations wrap the flat `api` object from @/lib/api. The shared axios layer
// in @/lib/http-client already toasts business errors ({success:false}) and
// HTTP failures, so these hooks keep their own error handling minimal: the
// mutationFn throws on a `success:false` body so useMutation transitions to
// error state (and so mutateAsync rejects in the form handler), while
// success-side cache invalidation + UI toasts live in the components.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import type {
  Account,
  AccountPayload,
  AccountStatus,
  AccountsSnapshot,
  LoginAccountPayload,
  Site,
} from './types'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const accountQueryKeys = {
  all: ['accounts'] as const,
  snapshot: () => [...accountQueryKeys.all, 'snapshot'] as const,
  /**
   * Prefix of every server-paged table cache produced by `page(…)`.
   *
   * `page` and `snapshot` are SIBLINGS under `all`, so neither is a prefix of
   * the other: anything that has to reach the /accounts table (invalidation,
   * optimistic patching) targets `all` or `pages()` — never `snapshot()`
   * alone, which the table does not read.
   */
  pages: () => [...accountQueryKeys.all, 'page'] as const,
  page: (
    pageIndex: number,
    pageSize: number,
    q?: string,
    status?: string,
    site?: string
  ) =>
    [
      ...accountQueryKeys.all,
      'page',
      {
        pageIndex,
        pageSize,
        q: q || undefined,
        status: status || undefined,
        site: site || undefined,
      },
    ] as const,
  detail: (id: number) => ['accounts', 'detail', id] as const,
}

// ---------------------------------------------------------------------------
// useAccountsPage — one server-side page of the accounts/sites projection
// ---------------------------------------------------------------------------

/** One page returned by GET /api/accounts?page=&pageSize=. */
export type AccountsPageData = {
  /** Legacy snapshot-shaped fixture compatibility; production pages use items. */
  accounts?: object[]
  items: object[]
  total: number
  sites: Site[]
  generatedAt: string
}

/**
 * Fetch + shape one server-side page. The page-gated endpoint keeps the same
 * account/site projection as the snapshot and slices before JSON encoding,
 * so large fleets no longer transfer every row to the browser. `q` / `status`
 * / `site` are pushed server-side (#1108): the filters match the whole fleet,
 * not just the loaded page. A missing or malformed total degrades to the
 * returned page length (the pager then shows one page rather than an
 * invented total).
 */
export async function fetchAccountsPage(params: {
  pageIndex: number
  pageSize: number
  q: string
  status: string
  site: string
}): Promise<AccountsPageData> {
  const response = await api.getAccountsPage({
    page: params.pageIndex + 1,
    pageSize: params.pageSize,
    q: params.q,
    status: params.status,
    site: params.site,
  })
  const items = Array.isArray(response.items)
    ? (response.items as object[])
    : []
  const sites = Array.isArray(response.sites) ? (response.sites as Site[]) : []
  return {
    items,
    sites,
    generatedAt:
      typeof response.generatedAt === 'string' ? response.generatedAt : '',
    total:
      typeof response.total === 'number' && Number.isFinite(response.total)
        ? response.total
        : items.length,
  }
}

/**
 * Fetch one server-side accounts page by URL-owned table state. The query is
 * keyed by pageIndex/pageSize AND the server-side filters (q/status/site), so
 * a filter change refetches a freshly-filtered page from the backend (#1108).
 */
export function useAccountsPage(
  params: {
    pageIndex: number
    pageSize: number
    q: string
    status: string
    site: string
  },
  options?: Omit<UseQueryOptions<AccountsPageData>, 'queryKey' | 'queryFn'>
) {
  return useQuery<AccountsPageData>({
    queryKey: accountQueryKeys.page(
      params.pageIndex,
      params.pageSize,
      params.q,
      params.status,
      params.site
    ),
    queryFn: () => fetchAccountsPage(params),
    placeholderData: (previous) => previous,
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useAccounts — snapshot list (accounts + sites + generatedAt)
// ---------------------------------------------------------------------------

export function useAccounts(
  options?: Omit<UseQueryOptions<AccountsSnapshot>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: accountQueryKeys.snapshot(),
    queryFn: async () => {
      const snapshot = await api.getAccountsSnapshot()
      if (!snapshot || !Array.isArray(snapshot.accounts)) {
        throw new Error('Failed to load accounts snapshot')
      }
      return snapshot as AccountsSnapshot
    },
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useCreateAccount — POST /api/accounts
// ---------------------------------------------------------------------------

/**
 * Post-create token sync statuses reported truthfully by the backend
 * (handler/admin/account_tokens_sync.go). `synced`/`empty`/`skipped` are
 * informational; `failed` means partial initialization — the account row is
 * persisted but its upstream tokens did not sync.
 */
export type TokenSyncStatus = 'synced' | 'empty' | 'failed' | 'skipped'

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

export function resolveCreatedAccountId(
  result: CreateAccountResult | undefined
): number | undefined {
  if (result?.id && result.id > 0) return result.id

  const createdItem = result?.items?.find(
    (item) => item.status === 'created' && item.id && item.id > 0
  )
  if (createdItem?.id) return createdItem.id

  return undefined
}

export function useCreateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: AccountPayload) => {
      const result = await api.addAccount(payload)
      return assertBusinessOk<CreateAccountResult>(
        result,
        'accounts.toast.createFailed'
      )
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      return data
    },
  })
}

// ---------------------------------------------------------------------------
// useLoginAccount — POST /api/accounts/login (password-mode binding)
// ---------------------------------------------------------------------------

export interface LoginAccountResult {
  success?: boolean
  message?: string
  account?: { id?: number; username?: string }
  reusedAccount?: boolean
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
}

export function useLoginAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: LoginAccountPayload) => {
      const result = await api.loginAccount(payload)
      return assertBusinessOk<LoginAccountResult>(
        result,
        'accounts.toast.loginFailed'
      )
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      return data
    },
  })
}

// ---------------------------------------------------------------------------
// useVerifyAccountToken — POST /api/accounts/verify-token (inline credential
// check before save). Passes skipErrorHandler so the shared axios layer does
// not toast a global error: the form renders the verified/failed state inline
// instead. The backend returns non-2xx {"error"} on failure, so assertBusinessOk
// only ever sees success envelopes here.
// ---------------------------------------------------------------------------

export interface VerifyTokenResult {
  tokenType?: string
  modelCount?: number
  models?: unknown[]
  userInfo?: unknown
  balance?: unknown
  apiToken?: string
  apiTokenFound?: boolean
}

export function useVerifyAccountToken() {
  return useMutation({
    mutationFn: async (payload: {
      siteId: number
      accessToken: string
      platformUserId?: number
      proxyUrl?: string
      credentialMode: 'session' | 'apikey'
    }) => {
      const result = await api.verifyToken(payload, { skipErrorHandler: true })
      return assertBusinessOk<VerifyTokenResult>(
        result,
        'accounts.verify.failed'
      )
    },
  })
}

// ---------------------------------------------------------------------------
// useUpdateAccount — PUT /api/accounts/:id
// ---------------------------------------------------------------------------

export function useUpdateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      payload,
    }: {
      id: number
      payload: AccountPayload
    }) => {
      const result = await api.updateAccount(id, payload)
      return assertBusinessOk(result, 'accounts.toast.updateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.updated'))
    },
  })
}

// ---------------------------------------------------------------------------
// useRefreshAccount — POST /api/accounts/:id/balance (per-row 余额刷新)
// ---------------------------------------------------------------------------

export function useRefreshAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.refreshBalance(id)
      return assertBusinessOk(result, 'accounts.toast.refreshFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.balanceRefreshed'))
    },
  })
}

// ---------------------------------------------------------------------------
// useDeleteAccount — DELETE /api/accounts/:id
// ---------------------------------------------------------------------------

export function useDeleteAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteAccount(id)
      return assertBusinessOk(result, 'accounts.toast.deleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.deleted'))
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchUpdateAccounts — POST /api/accounts/batch { ids, action }
// ---------------------------------------------------------------------------

export type BatchAccountAction =
  | 'enable'
  | 'disable'
  | 'delete'
  | 'refreshBalance'

export interface BatchAccountResult {
  successIds?: number[]
  failedItems?: Array<{ id: number; message?: string }>
}

export function useBatchUpdateAccounts() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      ids,
      action,
    }: {
      ids: number[]
      action: BatchAccountAction
    }) => {
      const result = await api.batchUpdateAccounts({ ids, action })
      return assertBusinessOk<BatchAccountResult>(
        result,
        'accounts.toast.batchFailed'
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      const failedItems = result?.failedItems ?? []
      if (failedItems.length > 0) {
        const failedList = failedItems.map((item) => `#${item.id}`).join(', ')
        toast.warning(
          i18n.t('accounts.toast.bulkPartial', {
            success: result?.successIds?.length ?? 0,
            failed: failedItems.length,
            items: failedList,
          })
        )
      }
    },
  })
}

// ---------------------------------------------------------------------------
// Field-level toggle mutations
//
// All three toggles run the oauth-connections optimistic pattern: onMutate
// snapshots every cache it is about to touch, patches the row in all of them
// (instant flip), onError restores each patched key from that snapshot, and
// onSettled invalidates so the server truth wins either way. The row stays
// interactive — the optimistic flip IS the feedback; the accounts page keeps
// its mutation-derived per-row pending spinner for the status toggle.
//
// The /accounts table reads `accountQueryKeys.page(…)`, so the PAGED caches are
// the ones that must flip; the snapshot is patched as well because it has its
// own mounted consumers (sites / check-in / routes / downstream-keys pages)
// rendering the same three fields.
// ---------------------------------------------------------------------------

type AccountToggleContext = {
  previousPages: Array<[readonly unknown[], AccountsPageData | undefined]>
  previousSnapshot: AccountsSnapshot | undefined
}

function isAccountRow(row: unknown, accountId: number): boolean {
  return (
    typeof row === 'object' &&
    row !== null &&
    Number((row as { id?: unknown }).id) === accountId
  )
}

/**
 * Flip one account row inside every cached page payload. Page rows are the raw
 * server objects the table parses per render (`parseAccountRow`), so the patch
 * builds a NEW row object — the WeakMap parse cache keys off object identity
 * and re-parses the patched copy instead of returning the stale parse.
 */
function patchAccountInPages(
  queryClient: QueryClient,
  accountId: number,
  patch: Partial<Account>
) {
  queryClient.setQueriesData<AccountsPageData>(
    { queryKey: accountQueryKeys.pages() },
    (current) => {
      if (!current) return current
      const patchRows = (rows: object[]): object[] =>
        rows.map((row) =>
          isAccountRow(row, accountId)
            ? { ...(row as Record<string, unknown>), ...patch }
            : row
        )
      return {
        ...current,
        // Legacy snapshot-shaped fixtures carry `accounts` instead of `items`
        // (the page reads `items ?? accounts`), so patch whichever exists.
        ...(Array.isArray(current.items)
          ? { items: patchRows(current.items) }
          : {}),
        ...(Array.isArray(current.accounts)
          ? { accounts: patchRows(current.accounts) }
          : {}),
      }
    }
  )
}

function patchAccountInSnapshot(
  queryClient: QueryClient,
  accountId: number,
  patch: Partial<Account>
) {
  queryClient.setQueryData<AccountsSnapshot>(
    accountQueryKeys.snapshot(),
    (current) =>
      current
        ? {
            ...current,
            accounts: current.accounts.map((account) =>
              account.id === accountId ? { ...account, ...patch } : account
            ),
          }
        : current
  )
}

/**
 * Cancel in-flight fetches for both caches, remember their current payloads
 * (per page key — there is one cache per filter/pagination combination) and
 * apply the optimistic patch. Returns the rollback context.
 */
async function beginAccountToggle(
  queryClient: QueryClient,
  accountId: number,
  patch: Partial<Account>
): Promise<AccountToggleContext> {
  await queryClient.cancelQueries({ queryKey: accountQueryKeys.pages() })
  await queryClient.cancelQueries({ queryKey: accountQueryKeys.snapshot() })
  const previousPages = queryClient.getQueriesData<AccountsPageData>({
    queryKey: accountQueryKeys.pages(),
  })
  const previousSnapshot = queryClient.getQueryData<AccountsSnapshot>(
    accountQueryKeys.snapshot()
  )
  patchAccountInPages(queryClient, accountId, patch)
  patchAccountInSnapshot(queryClient, accountId, patch)
  return { previousPages, previousSnapshot }
}

function rollbackAccountToggle(
  queryClient: QueryClient,
  context: AccountToggleContext | undefined
) {
  for (const [queryKey, previous] of context?.previousPages ?? []) {
    queryClient.setQueryData(queryKey, previous)
  }
  if (context?.previousSnapshot) {
    queryClient.setQueryData(
      accountQueryKeys.snapshot(),
      context.previousSnapshot
    )
  }
}

export function useToggleAccountPin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, isPinned }: { id: number; isPinned: boolean }) => {
      const result = await api.updateAccount(id, { isPinned })
      return assertBusinessOk(result, 'accounts.toast.pinFailed')
    },
    onMutate: ({ id, isPinned }) =>
      beginAccountToggle(queryClient, id, { isPinned }),
    onError: (_error, _variables, context) => {
      rollbackAccountToggle(queryClient, context)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

export function useToggleAccountStatus() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      status,
    }: {
      id: number
      status: AccountStatus
    }) => {
      const result = await api.updateAccount(id, { status })
      return assertBusinessOk(result, 'accounts.toast.statusFailed')
    },
    onMutate: ({ id, status }) =>
      beginAccountToggle(queryClient, id, { status }),
    onError: (_error, _variables, context) => {
      rollbackAccountToggle(queryClient, context)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

export function useToggleAccountCheckin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      checkinEnabled,
    }: {
      id: number
      checkinEnabled: boolean
    }) => {
      const result = await api.updateAccount(id, { checkinEnabled })
      return assertBusinessOk(result, 'accounts.toast.checkinFailed')
    },
    onMutate: ({ id, checkinEnabled }) =>
      beginAccountToggle(queryClient, id, { checkinEnabled }),
    onError: (_error, _variables, context) => {
      rollbackAccountToggle(queryClient, context)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

// ---------------------------------------------------------------------------
// Convenience selector
// ---------------------------------------------------------------------------

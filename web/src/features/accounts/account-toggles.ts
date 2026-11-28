import {
  useMutation,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'

import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'

import { accountQueryKeys, type AccountsPageData } from './account-queries'
import type { Account, AccountStatus, AccountsSnapshot } from './types'

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

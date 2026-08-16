// metapi-go/routes — accounts list.
//
// The route validates the canonical accounts table URL state. The feature page
// consumes the same schema through the shared URL-table adapter, so search,
// filters, sorting and pagination have one source of truth instead of being
// mirrored into local state and written back from an effect.
//
// `loader` prefetches the accounts snapshot (`accountQueryKeys.snapshot()`),
// which the backend returns as `{ accounts, sites, generatedAt }` — the
// embedded `sites` array powers the page's site filter, so a single prefetch
// covers both. The component is declared directly; the router plugin's
// `autoCodeSplitting` splits it in production.

import { createFileRoute } from '@tanstack/react-router'

import { accountQueryKeys } from '@/features/accounts'
import { AccountsPage } from '@/features/accounts/components/accounts-page'
import { accountsSearchSchema } from '@/features/accounts/lib/accounts-search-schema'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/accounts')({
  validateSearch: accountsSearchSchema,
  staticData: { title: 'accounts.page.title' },
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: accountQueryKeys.snapshot(),
      queryFn: () => api.getAccountsSnapshot(),
    })
  },
  component: AccountsPage,
})

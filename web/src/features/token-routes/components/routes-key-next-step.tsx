// metapi-go features/token-routes/components — journey step 3 → 4 handoff strip.
//
// Routes alone do not make /v1 callable; a downstream key does. Nothing on the
// routes page said so, which left the guided site → account → route chain
// ending at a dead stop (audit: zero downstream-keys references reachable from
// token-routes). Step 1 hands off through SiteCreatedModal, step 2 through
// showAccountCreatedToast, step 3 through showRouteCompletionToast — this is
// the persistent counterpart for an operator who built routes earlier and
// never issued a key.
//
// It renders only while the gap is real, so it stays a targeted handoff rather
// than a banner: the host mounts it only when routes exist (the empty state's
// correct next step is "add a route"), and the strip itself stays silent while
// the key count is loading, when the count failed to load, and forever after
// the first key is issued.
//
// The query reuses the downstream-keys section's own key + queryFn shape, so
// the count is a cache hit for anyone who has already opened Downstream Keys
// and warms that cache for the click-through.

import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { KeyRound } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  downstreamKeysQueryKeys,
  type DownstreamKeysResponse,
} from '@/features/settings/sections/downstream/components/key-form-shared'
import { api } from '@/lib/api'

export function RoutesKeyNextStep() {
  const { t } = useTranslation()

  const keysQuery = useQuery({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 60 * 1000,
  })

  // Only a RESOLVED count of exactly zero earns the nudge: `undefined`
  // (loading or errored) must never be read as "no keys issued".
  if (keysQuery.data?.items.length !== 0) return null

  return (
    <div className='bg-muted/40 text-muted-foreground flex flex-wrap items-center justify-between gap-2 rounded-lg border p-2 text-sm'>
      <span>{t('tokenRoutes.page.nextStepBody')}</span>
      <Button
        variant='outline'
        size='sm'
        render={<Link to='/downstream-keys' />}
      >
        <KeyRound className='size-4' />
        {t('tokenRoutes.page.nextStepAction')}
      </Button>
    </div>
  )
}

import { useTranslation } from 'react-i18next'

import { toast } from '@/lib/toast'

import { useUpdateSite } from '../api'
import { useSitesColumns, type SitesColumnActions } from './sites-columns'

export function useSiteRowColumns(
  dialogs: Pick<SitesColumnActions, 'onEdit' | 'onView' | 'onDelete'>
) {
  const { t } = useTranslation()
  const updateSite = useUpdateSite()
  // Per-row pending state for the inline status/pin toggles: only the row
  // whose update is in flight disables those actions and shows a spinner.
  const pendingSiteId = updateSite.isPending
    ? (updateSite.variables?.id ?? null)
    : null

  return useSitesColumns(
    {
      ...dialogs,
      onToggleStatus: (site) => {
        const nextStatus = site.status === 'disabled' ? 'active' : 'disabled'
        updateSite.mutate(
          { id: site.id, payload: { status: nextStatus } },
          {
            onSuccess: () =>
              toast.success(
                t('sites.toast.statusToggled', { name: site.name })
              ),
          }
        )
      },
      onTogglePin: (site) => {
        updateSite.mutate(
          { id: site.id, payload: { isPinned: !site.isPinned } },
          {
            onSuccess: () =>
              toast.success(t('sites.toast.pinToggled', { name: site.name })),
          }
        )
      },
    },
    pendingSiteId
  )
}

import { useNavigate } from '@tanstack/react-router'
import axios from 'axios'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useRebuildRoutes } from '@/features/token-routes'

import { useImportSites } from '../api'
import type { ImportSitesResult, ImportSiteItem } from '../types'

export function useImportExecution(
  onImported: () => void,
  onClose: () => void
) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const importSites = useImportSites()
  const rebuildRoutes = useRebuildRoutes()
  const [result, setResult] = useState<ImportSitesResult | null>(null)
  async function submit(items: ImportSiteItem[]) {
    try {
      const importResult = await importSites.mutateAsync({
        items,
        duplicateStrategy: 'skip',
      })
      setResult(importResult)
      onImported()
    } catch (error) {
      // Transport failures (non-2xx) are toasted by the http-client error
      // interceptor; a plain error thrown by the mutation (e.g. an empty
      // response body) is not, so surface it instead of swallowing.
      if (!axios.isAxiosError(error)) {
        toast.error(t('import.submitFailed'))
      }
    }
  }
  function finish() {
    setResult(null)
    onClose()
  }
  // Imported sites are not routable until a route rebuild picks them up.
  // Reuse the shared rebuild mutation (it already toasts queued/complete
  // outcomes); close the wizard once the rebuild request lands. On failure
  // the wizard stays open so the operator can retry.
  function handleRebuildRoutes() {
    rebuildRoutes.mutate(undefined, {
      onSuccess: finish,
    })
  }

  // Mirror the manual site-created modal's exit (site-created-modal.tsx):
  // land the accounts page with the one-shot `siteId`/`create` deep link so
  // the create dialog opens with the imported site preselected. Exactly one
  // imported/merged row with a siteId carries it; any other successful
  // batch at least opens the create flow; an all-skipped/failed batch keeps
  // the plain navigation (nothing to preselect).
  function handleGoToAccounts() {
    const results = result?.results ?? []
    const successful = results.filter(
      (item) => item.status === 'imported' || item.status === 'merged'
    )
    const withSiteId = successful.filter(
      (item) => typeof item.siteId === 'number' && item.siteId > 0
    )
    finish()
    if (successful.length === 1 && withSiteId.length === 1) {
      navigate({
        to: '/accounts',
        search: { siteId: withSiteId[0].siteId, create: true },
        replace: true,
      })
      return
    }
    if (successful.length > 0) {
      navigate({
        to: '/accounts',
        search: { create: true },
        replace: true,
      })
      return
    }
    navigate({ to: '/accounts' })
  }
  return {
    result,
    submit,
    finish,
    handleRebuildRoutes,
    handleGoToAccounts,
    isSubmitting: importSites.isPending,
    isRebuilding: rebuildRoutes.isPending,
  }
}

import { useNavigate } from '@tanstack/react-router'
import axios from 'axios'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Command, CommandDialog, CommandInput } from '@/components/ui/command'
import { useSearchActions } from '@/hooks/use-search-actions'
import { searchApi, type SearchResponse } from '@/lib/api/search'
import { toast } from '@/lib/toast'

import type { SearchActionId } from './lib/search-actions'
import { entitySearchGroups } from './search-entity-groups'
import { useLocalSearchGroups } from './search-local-groups'
import { SearchResultList, type SearchTarget } from './search-result-list'

const SEARCH_DEBOUNCE_MS = 250

type SearchModalProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

function isEditableTarget(target: HTMLElement): boolean {
  const tagName = target.tagName
  if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT') {
    return true
  }
  return target.isContentEditable
}

export function SearchModal(props: SearchModalProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const [query, setQuery] = React.useState('')
  const [results, setResults] = React.useState<SearchResponse | null>(null)
  const [isSearching, setIsSearching] = React.useState(false)

  const trimmedQuery = query.trim()

  // Keep the latest open state in a ref so the global shortcut listener is
  // registered exactly once and still sees fresh values.
  const openRef = React.useRef(props.open)
  const onOpenChangeRef = React.useRef(props.onOpenChange)

  React.useEffect(() => {
    openRef.current = props.open
    onOpenChangeRef.current = props.onOpenChange
  })

  // Global Ctrl/Cmd+K toggle. Skipped inside editable targets so the
  // shortcut never hijacks typing in inputs, textareas or the palette's own
  // search field.
  React.useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.repeat || event.key.toLowerCase() !== 'k') return
      if (!event.metaKey && !event.ctrlKey) return
      if (
        event.target instanceof HTMLElement &&
        isEditableTarget(event.target)
      ) {
        return
      }
      event.preventDefault()
      onOpenChangeRef.current(!openRef.current)
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [])

  // Reset transient state whenever the palette closes so the next open
  // starts from the quick-entry state instead of stale results.
  React.useEffect(() => {
    if (!props.open) {
      setQuery('')
      setResults(null)
      setIsSearching(false)
    }
  }, [props.open])

  // Debounced backend search. The cleanup aborts in-flight requests and
  // drops stale responses when the query changes or the palette unmounts.
  React.useEffect(() => {
    if (!trimmedQuery) {
      setResults(null)
      setIsSearching(false)
      return
    }

    const controller = new AbortController()
    let disposed = false

    const timeoutId = window.setTimeout(() => {
      setIsSearching(true)
      searchApi
        .search(trimmedQuery, { signal: controller.signal })
        .then((response) => {
          if (!disposed) setResults(response)
        })
        .finally(() => {
          if (!disposed) setIsSearching(false)
        })
        .catch(() => {
          // Failures are toasted by the shared http-client error handler;
          // reset to the empty state here.
          if (!disposed) setResults(null)
        })
    }, SEARCH_DEBOUNCE_MS)

    return () => {
      disposed = true
      controller.abort()
      window.clearTimeout(timeoutId)
    }
  }, [trimmedQuery])

  // Navigation targets come from the validated nav config (root sidebar +
  // settings registry) and the log deep links below, which the generated
  // route tree cannot enumerate — cast once here instead of per call site.
  const closeAndNavigate = React.useCallback(
    (target: SearchTarget) => {
      props.onOpenChange(false)
      navigate(target as unknown as Parameters<typeof navigate>[0])
    },
    [navigate, props]
  )

  const { triggerAllCheckin, rebuildRoutes, refreshRouteDecisions } =
    useSearchActions()
  const [rebuildConfirmOpen, setRebuildConfirmOpen] = React.useState(false)

  // Mirrors the checkin page's "Run all check-ins" handler: same mutation,
  // same honest summary toast (partial failures reported as errors, not
  // swallowed). The palette closes first; the mutation + toast complete in
  // the background while the user is back on the page.
  const runCheckinAll = React.useCallback(async () => {
    props.onOpenChange(false)
    try {
      const result = await triggerAllCheckin.mutateAsync()
      const summary = result.summary
      if (summary) {
        const description = t('checkin.toast.summary', {
          total: summary.total,
          success: summary.success,
          failed: summary.failed,
          skipped: summary.skipped,
        })
        if (summary.failed > 0) {
          toast.error(t('checkin.toast.partialFailed'), { description })
        } else {
          toast.success(t('checkin.toast.complete'), { description })
        }
      } else {
        toast.success(result.message || t('checkin.toast.complete'))
      }
    } catch (error) {
      // Transport failures (non-2xx) are already toasted by the http-client
      // error interceptor; envelope failures thrown by the parser are not,
      // so they get their own honest error toast.
      if (!axios.isAxiosError(error)) {
        toast.error(
          error instanceof Error && error.message
            ? error.message
            : t('checkin.toast.triggerFailed')
        )
      }
    }
  }, [props, t, triggerAllCheckin])

  const executeAction = React.useCallback(
    (id: SearchActionId) => {
      switch (id) {
        case 'add-site':
          closeAndNavigate({ to: '/sites', search: { create: true } })
          return
        case 'run-checkin-all':
          void runCheckinAll()
          return
        case 'rebuild-routes':
          // The routes page gates this mutation behind a ConfirmDialog; the
          // palette keeps the same discipline with the same wording.
          props.onOpenChange(false)
          setRebuildConfirmOpen(true)
          return
        case 'refresh-route-decisions':
          props.onOpenChange(false)
          refreshRouteDecisions.mutate()
          return
      }
    },
    [closeAndNavigate, props, refreshRouteDecisions, runCheckinAll]
  )

  const localGroups = useLocalSearchGroups(trimmedQuery)
  const entityGroups = React.useMemo(
    () => entitySearchGroups(results, trimmedQuery, t),
    [results, trimmedQuery, t]
  )

  return (
    <>
      <CommandDialog
        modal
        open={props.open}
        onOpenChange={props.onOpenChange}
        title={t('search.title')}
        description={t('search.description')}
        className='sm:max-w-xl'
      >
        <Command shouldFilter={false}>
          <CommandInput
            autoFocus
            value={query}
            onValueChange={setQuery}
            placeholder={t('search.placeholder')}
            aria-label={t('search.title')}
          />
          <SearchResultList
            query={trimmedQuery}
            groups={[...localGroups, ...entityGroups]}
            isSearching={isSearching}
            onNavigate={closeAndNavigate}
            onAction={executeAction}
          />
        </Command>
      </CommandDialog>
      {/* Confirmation for the rebuild-routes action. Same wording as the
          routes page's own gate; the palette is already closed at this
          point, so the dialog takes over the screen alone. */}
      <ConfirmDialog
        open={rebuildConfirmOpen}
        title={t('tokenRoutes.page.rebuildConfirmTitle')}
        description={t('tokenRoutes.page.rebuildConfirmDescription')}
        confirmLabel={t('tokenRoutes.page.rebuild')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setRebuildConfirmOpen(false)
          rebuildRoutes.mutate({ refreshModels: true })
        }}
        onCancel={() => setRebuildConfirmOpen(false)}
      />
    </>
  )
}

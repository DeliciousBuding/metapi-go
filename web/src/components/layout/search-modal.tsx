// metapi-go/layout — global search command palette (⌘K / Ctrl+K).
//
// A cmdk-based palette with three result layers:
//   - a local navigation layer over the primary pages (root sidebar data)
//     and the Settings workspace (5 subareas + sections from the settings
//     registry): quick entries when the query is empty, local fuzzy
//     matching while typing,
//   - a local actions layer over high-frequency write operations
//     (registry in `lib/search-actions.ts`): every action reuses an
//     existing dialog deep link or page mutation — no new business logic,
//   - the backend entity layer over `POST /api/search` (six categories).
//
// The trigger button lives in `app-header.tsx`; this component is mounted
// once in `AuthenticatedLayout` (inside the router, because result clicks
// use `useNavigate`) and owns:
//   - the global ⌘K/Ctrl+K toggle (registered once, never hijacked inside
//     editable targets),
//   - a ~250 ms debounced search against `searchApi.search`,
//   - grouped rendering of navigation + actions + entity results.
//
// cmdk's own filtering is disabled (`shouldFilter={false}`): results are
// already filtered locally / by the backend, so empty/loading states are
// rendered explicitly instead of relying on cmdk's Empty semantics.

import { useNavigate } from '@tanstack/react-router'
import axios from 'axios'
import {
  Boxes,
  CalendarCheck,
  Globe,
  KeyRound,
  LayoutGrid,
  ScrollText,
  Search as SearchIcon,
  Settings,
  UserRound,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import {
  Command,
  CommandDialog,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Spinner } from '@/components/ui/spinner'
import { useSearchActions } from '@/hooks/use-search-actions'
import { useSidebarData } from '@/hooks/use-sidebar-data'
import {
  searchApi,
  type SearchAccount,
  type SearchAccountToken,
  type SearchCheckinLog,
  type SearchModel,
  type SearchProxyLog,
  type SearchResponse,
  type SearchSite,
} from '@/lib/api/search'
import { toast } from '@/lib/toast'

import {
  matchActionEntries,
  SEARCH_ACTION_ENTRIES,
  type SearchActionId,
} from './lib/search-actions'
import {
  getSettingsNavEntries,
  matchNavEntries,
  pageEntriesFromNavGroups,
  type SearchNavEntry,
} from './lib/search-nav'

const SEARCH_DEBOUNCE_MS = 250
const MAX_ITEMS_PER_GROUP = 8

type SearchModalProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

type SearchItem = {
  key: string
  label: string
  description: string | null
  icon?: React.ElementType
  onSelect: () => void
}

type SearchGroup = {
  key: string
  heading: string
  icon: LucideIcon
  items: SearchItem[]
}

function isEditableTarget(target: HTMLElement): boolean {
  const tagName = target.tagName
  if (tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT') {
    return true
  }
  return target.isContentEditable
}

function firstNonEmpty(
  ...values: Array<string | null | undefined>
): string | null {
  for (const value of values) {
    const trimmed = value?.trim()
    if (trimmed) return trimmed
  }
  return null
}

export function SearchModal(props: SearchModalProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const sidebarData = useSidebarData()

  const [query, setQuery] = React.useState('')
  const [results, setResults] = React.useState<SearchResponse | null>(null)
  const [isSearching, setIsSearching] = React.useState(false)

  const trimmedQuery = query.trim()
  const hasQuery = trimmedQuery.length > 0

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
    (target: { to: string; search?: Record<string, unknown> }) => {
      props.onOpenChange(false)
      navigate(target as unknown as Parameters<typeof navigate>[0])
    },
    [navigate, props]
  )

  const navigateToQueryPage = React.useCallback(
    (to: '/sites' | '/accounts' | '/models') => {
      closeAndNavigate({ to, search: { q: trimmedQuery } })
    },
    [closeAndNavigate, trimmedQuery]
  )

  // Check-in log hits deep-link with the filters the /checkin schema
  // supports: the owning account (when the backend returned it) plus the
  // typed text, which the page matches against username / site / message.
  const navigateToCheckin = React.useCallback(
    (log: SearchCheckinLog) => {
      const search: { accountId?: number; q?: string } = {}
      if (log.accountId != null) search.accountId = log.accountId
      if (trimmedQuery) search.q = trimmedQuery
      closeAndNavigate({ to: '/checkin', search })
    },
    [closeAndNavigate, trimmedQuery]
  )

  // Proxy log hits go straight to the dedicated /proxy-logs workspace with
  // the matched model as the text filter (and the status filter when the
  // log carries one the page supports).
  const navigateToProxyLogs = React.useCallback(
    (log: SearchProxyLog) => {
      const searchText = firstNonEmpty(log.modelRequested, trimmedQuery)
      const search: { q?: string; status?: 'success' | 'failed' } = {}
      if (searchText) search.q = searchText
      if (log.status === 'success' || log.status === 'failed') {
        search.status = log.status
      }
      closeAndNavigate({ to: '/proxy-logs', search })
    },
    [closeAndNavigate, trimmedQuery]
  )

  // ---- Actions layer -------------------------------------------------------
  //
  // High-frequency write actions (registry: lib/search-actions.ts). Every
  // action reuses an affordance the pages already expose: the add-site /
  // add-site navigates the one-shot `?create=1` deep link the sites page
  // consumes (same path the dashboard onboarding CTA writes), and the
  // operational entries fire the same mutation hooks the page buttons use.

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

  const actionGroups = React.useMemo((): SearchGroup[] => {
    const matched = hasQuery
      ? matchActionEntries(SEARCH_ACTION_ENTRIES, trimmedQuery, (key) => t(key))
      : [...SEARCH_ACTION_ENTRIES]
    if (matched.length === 0) return []
    return [
      {
        key: 'actions',
        heading: t('search.groups.actions'),
        icon: Zap,
        items: matched.map((entry) => ({
          key: `action-${entry.id}`,
          label: t(entry.titleKey),
          description: null,
          icon: entry.icon,
          onSelect: () => executeAction(entry.id),
        })),
      },
    ]
  }, [hasQuery, trimmedQuery, t, executeAction])

  // ---- Local navigation layer --------------------------------------------

  const pageEntries = React.useMemo(
    () => pageEntriesFromNavGroups(sidebarData.navGroups),
    [sidebarData]
  )
  const settingsEntries = React.useMemo(() => getSettingsNavEntries(), [])

  const navigationGroups = React.useMemo((): SearchGroup[] => {
    // Quick entries (empty query) list every page/subarea; typed matches are
    // capped so the mixed navigation + entity list stays scannable.
    const toItems = (entries: SearchNavEntry[], cap?: number): SearchItem[] =>
      (cap ? entries.slice(0, cap) : entries).map((entry) => ({
        key: entry.key,
        label: t(entry.titleKey),
        description: null,
        ...(entry.icon ? { icon: entry.icon } : {}),
        onSelect: () => closeAndNavigate({ to: entry.url }),
      }))

    if (!hasQuery) {
      // Quick entries: primary pages + the 5 settings subareas.
      return [
        {
          key: 'nav-pages',
          heading: t('search.groups.pages'),
          icon: LayoutGrid,
          items: toItems(pageEntries),
        },
        {
          key: 'nav-settings',
          heading: t('search.groups.settings'),
          icon: Settings,
          items: toItems(
            settingsEntries.filter((entry) =>
              entry.key.startsWith('settings-subarea-')
            )
          ),
        },
      ]
    }

    const resolveLabel = (titleKey: string) => t(titleKey)
    const matchedPages = matchNavEntries(
      pageEntries,
      trimmedQuery,
      resolveLabel
    )
    const matchedSettings = matchNavEntries(
      settingsEntries,
      trimmedQuery,
      resolveLabel
    )

    const groups: SearchGroup[] = []
    if (matchedPages.length > 0) {
      groups.push({
        key: 'nav-pages',
        heading: t('search.groups.pages'),
        icon: LayoutGrid,
        items: toItems(matchedPages, MAX_ITEMS_PER_GROUP),
      })
    }
    if (matchedSettings.length > 0) {
      groups.push({
        key: 'nav-settings',
        heading: t('search.groups.settings'),
        icon: Settings,
        items: toItems(matchedSettings, MAX_ITEMS_PER_GROUP),
      })
    }
    return groups
  }, [
    hasQuery,
    trimmedQuery,
    t,
    pageEntries,
    settingsEntries,
    closeAndNavigate,
  ])

  // ---- Backend entity layer ----------------------------------------------

  const entityGroups = React.useMemo(() => {
    if (!results) return []

    const unknownLabel = t('search.unknown')

    const siteItems: SearchItem[] = (results.sites ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((site: SearchSite) => ({
        key: `site-${site.id}`,
        label: firstNonEmpty(site.name) ?? unknownLabel,
        description: firstNonEmpty(site.url),
        // Deep-link straight to the entity: the sites page consumes the
        // one-shot `?edit=<id>` param (same channel as its row action).
        onSelect: () =>
          closeAndNavigate({ to: '/sites', search: { edit: site.id } }),
      }))

    const accountItems: SearchItem[] = (results.accounts ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((account: SearchAccount) => ({
        key: `account-${account.id}`,
        label: firstNonEmpty(account.username) ?? unknownLabel,
        description: firstNonEmpty(account.siteName),
        // The accounts page consumes the one-shot `?accountId=<id>` param
        // (same channel as the dashboard attention deep link).
        onSelect: () =>
          closeAndNavigate({
            to: '/accounts',
            search: { accountId: account.id },
          }),
      }))

    const tokenItems: SearchItem[] = (results.accountTokens ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((token: SearchAccountToken) => ({
        key: `token-${token.id}`,
        label: firstNonEmpty(token.name) ?? unknownLabel,
        description: firstNonEmpty(token.accountUsername),
        // Jump to the owning account's detail sheet when the backend
        // returned its id; fall back to the q-filtered list for legacy
        // payloads without it.
        onSelect: () =>
          token.accountId != null
            ? closeAndNavigate({
                to: '/accounts',
                search: { accountId: token.accountId },
              })
            : navigateToQueryPage('/accounts'),
      }))

    const checkinItems: SearchItem[] = (results.checkinLogs ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((log: SearchCheckinLog) => ({
        key: `checkin-${log.id}`,
        label:
          firstNonEmpty(log.message, log.reward, log.status) ?? unknownLabel,
        description: firstNonEmpty(log.accountUsername),
        onSelect: () => navigateToCheckin(log),
      }))

    const proxyLogItems: SearchItem[] = (results.proxyLogs ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((log: SearchProxyLog) => ({
        key: `proxy-log-${log.id}`,
        label:
          firstNonEmpty(log.modelRequested, log.modelActual) ?? unknownLabel,
        description: firstNonEmpty(log.status),
        onSelect: () => navigateToProxyLogs(log),
      }))

    const modelItems: SearchItem[] = (results.models ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((model: SearchModel) => ({
        key: `model-${model.modelName}`,
        label: firstNonEmpty(model.modelName) ?? unknownLabel,
        description:
          model.tokenCount != null
            ? t('search.modelTokens', { count: model.tokenCount })
            : null,
        // The models page consumes the one-shot `?model=<name>` param and
        // opens the detail sheet.
        onSelect: () =>
          closeAndNavigate({
            to: '/models',
            search: { model: model.modelName },
          }),
      }))

    const builtGroups: SearchGroup[] = [
      {
        key: 'sites',
        heading: t('search.groups.sites'),
        icon: Globe,
        items: siteItems,
      },
      {
        key: 'accounts',
        heading: t('search.groups.accounts'),
        icon: UserRound,
        items: accountItems,
      },
      {
        key: 'accountTokens',
        heading: t('search.groups.tokens'),
        icon: KeyRound,
        items: tokenItems,
      },
      {
        key: 'checkinLogs',
        heading: t('search.groups.checkinLogs'),
        icon: CalendarCheck,
        items: checkinItems,
      },
      {
        key: 'proxyLogs',
        heading: t('search.groups.proxyLogs'),
        icon: ScrollText,
        items: proxyLogItems,
      },
      {
        key: 'models',
        heading: t('search.groups.models'),
        icon: Boxes,
        items: modelItems,
      },
    ]

    return builtGroups.filter((group) => group.items.length > 0)
  }, [
    results,
    t,
    closeAndNavigate,
    navigateToCheckin,
    navigateToProxyLogs,
    navigateToQueryPage,
  ])

  const groups = [...navigationGroups, ...actionGroups, ...entityGroups]
  const showEmpty = hasQuery && !isSearching && groups.length === 0

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
          <CommandList className='max-h-[min(60svh,28rem)]'>
            {isSearching && (
              <div className='text-muted-foreground flex items-center justify-center gap-2 px-2 py-4 text-sm'>
                <Spinner />
                <span>{t('search.searching')}</span>
              </div>
            )}
            {showEmpty && (
              <div className='flex flex-col items-center gap-1 px-4 py-10 text-center'>
                <SearchIcon className='text-muted-foreground/60 size-5' />
                <p className='text-sm font-medium'>
                  {t('search.noResults.title')}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t('search.noResults.description', { query: trimmedQuery })}
                </p>
              </div>
            )}
            {groups.map((group) => (
              <CommandGroup key={group.key} heading={group.heading}>
                {group.items.map((item) => {
                  const ItemIcon = item.icon ?? group.icon
                  return (
                    <CommandItem
                      key={item.key}
                      value={item.key}
                      onSelect={item.onSelect}
                    >
                      <ItemIcon className='text-muted-foreground size-4 shrink-0' />
                      <span className='flex min-w-0 flex-1 flex-col'>
                        <span className='truncate'>{item.label}</span>
                        {item.description ? (
                          <span className='text-muted-foreground truncate text-xs'>
                            {item.description}
                          </span>
                        ) : null}
                      </span>
                    </CommandItem>
                  )
                })}
              </CommandGroup>
            ))}
          </CommandList>
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

// metapi-go/layout — global search command palette (⌘K / Ctrl+K).
//
// A cmdk-based palette over the backend `POST /api/search` endpoint. The
// trigger button lives in `app-header.tsx`; this component is mounted once
// in `AuthenticatedLayout` (inside the router, because result clicks use
// `useNavigate`) and owns:
//   - the global ⌘K/Ctrl+K toggle (registered once, never hijacked inside
//     editable targets),
//   - a ~250 ms debounced search against `searchApi.search`,
//   - grouped rendering of the six result categories (8 items max each).
//
// cmdk's own filtering is disabled (`shouldFilter={false}`): results are
// already filtered by the backend, so empty/loading/hint states are rendered
// explicitly instead of relying on cmdk's Empty semantics.

import { useNavigate } from '@tanstack/react-router'
import {
  Boxes,
  CalendarCheck,
  Globe,
  KeyRound,
  ScrollText,
  Search as SearchIcon,
  UserRound,
  type LucideIcon,
} from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import {
  Command,
  CommandDialog,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Spinner } from '@/components/ui/spinner'
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
  // starts from the hint state instead of stale results.
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

  const navigateToQueryPage = React.useCallback(
    (to: '/sites' | '/accounts' | '/models') => {
      props.onOpenChange(false)
      navigate({ to, search: { q: trimmedQuery } })
    },
    [navigate, props, trimmedQuery]
  )

  const navigateToCheckin = React.useCallback(() => {
    props.onOpenChange(false)
    navigate({ to: '/checkin' })
  }, [navigate, props])

  const navigateToProxyLogs = React.useCallback(() => {
    props.onOpenChange(false)
    navigate({ to: '/observability', search: { section: 'proxy-logs' } })
  }, [navigate, props])

  const groups = React.useMemo(() => {
    if (!results) return []

    const unknownLabel = t('search.unknown')

    const siteItems: SearchItem[] = (results.sites ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((site: SearchSite) => ({
        key: `site-${site.id}`,
        label: firstNonEmpty(site.name) ?? unknownLabel,
        description: firstNonEmpty(site.url),
        onSelect: () => navigateToQueryPage('/sites'),
      }))

    const accountItems: SearchItem[] = (results.accounts ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((account: SearchAccount) => ({
        key: `account-${account.id}`,
        label: firstNonEmpty(account.username) ?? unknownLabel,
        description: firstNonEmpty(account.siteName),
        onSelect: () => navigateToQueryPage('/accounts'),
      }))

    const tokenItems: SearchItem[] = (results.accountTokens ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((token: SearchAccountToken) => ({
        key: `token-${token.id}`,
        label: firstNonEmpty(token.name) ?? unknownLabel,
        description: firstNonEmpty(token.accountUsername),
        onSelect: () => navigateToQueryPage('/accounts'),
      }))

    const checkinItems: SearchItem[] = (results.checkinLogs ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((log: SearchCheckinLog) => ({
        key: `checkin-${log.id}`,
        label:
          firstNonEmpty(log.message, log.reward, log.status) ?? unknownLabel,
        description: firstNonEmpty(log.accountUsername),
        onSelect: navigateToCheckin,
      }))

    const proxyLogItems: SearchItem[] = (results.proxyLogs ?? [])
      .slice(0, MAX_ITEMS_PER_GROUP)
      .map((log: SearchProxyLog) => ({
        key: `proxy-log-${log.id}`,
        label:
          firstNonEmpty(log.modelRequested, log.modelActual) ?? unknownLabel,
        description: firstNonEmpty(log.status),
        onSelect: navigateToProxyLogs,
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
        onSelect: () => navigateToQueryPage('/models'),
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
  }, [results, t, navigateToCheckin, navigateToProxyLogs, navigateToQueryPage])

  const showHint = !hasQuery && !isSearching
  const showEmpty = hasQuery && !isSearching && groups.length === 0

  return (
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
        />
        <CommandList className='max-h-[min(60svh,28rem)]'>
          {isSearching && (
            <div className='text-muted-foreground flex items-center justify-center gap-2 px-2 py-4 text-sm'>
              <Spinner />
              <span>{t('search.searching')}</span>
            </div>
          )}
          {showHint && (
            <div className='flex flex-col items-center gap-1 px-4 py-10 text-center'>
              <SearchIcon className='text-muted-foreground/60 size-5' />
              <p className='text-sm font-medium'>{t('search.empty.title')}</p>
              <p className='text-muted-foreground text-xs'>
                {t('search.empty.description')}
              </p>
            </div>
          )}
          {showEmpty && (
            <div className='flex flex-col items-center gap-1 px-4 py-10 text-center'>
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
              {group.items.map((item) => (
                <CommandItem
                  key={item.key}
                  value={item.key}
                  onSelect={item.onSelect}
                >
                  <group.icon className='text-muted-foreground size-4 shrink-0' />
                  <span className='flex min-w-0 flex-1 flex-col'>
                    <span className='truncate'>{item.label}</span>
                    {item.description ? (
                      <span className='text-muted-foreground truncate text-xs'>
                        {item.description}
                      </span>
                    ) : null}
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
        </CommandList>
      </Command>
    </CommandDialog>
  )
}

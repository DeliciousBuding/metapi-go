// metapi-go/features/channels — TanStack Query hooks wrapping `lib/api.ts`.
// The channels page is server-paginated (`GET /api/channels?page=&pageSize=`)
// and the error banner uses the independent fleet-wide error-summary endpoint
// so the count remains truthful even when only one page is loaded.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  channelsKeys,
  type ChannelRow,
  type ChannelsErrorSummary,
  type ChannelsPageData,
} from './types'

/**
 * Parse the legacy/full GET /api/channels envelope. Returns the `items` array
 * when the body is a well-formed `{items: [...]}` object; throws otherwise so
 * a malformed body fails the query explicitly instead of masquerading as an
 * empty list.
 */
export function parseChannelsEnvelope(result: unknown): ChannelRow[] {
  if (result && typeof result === 'object') {
    const items = (result as { items?: unknown }).items
    if (Array.isArray(items)) return items as ChannelRow[]
  }
  throw new Error('Invalid channels response')
}

/** Parse one server-side channels page envelope. */
export function parseChannelsPageEnvelope(result: unknown): ChannelsPageData {
  if (!result || typeof result !== 'object') {
    throw new Error('Invalid channels page response')
  }
  const envelope = result as { items?: unknown; total?: unknown }
  if (!Array.isArray(envelope.items)) {
    throw new Error('Invalid channels page response')
  }
  let total: number
  if (envelope.total === undefined) {
    // Backward-compatible fallback for older payloads that omit the count.
    total = envelope.items.length
  } else if (
    typeof envelope.total === 'number' &&
    Number.isFinite(envelope.total)
  ) {
    total = envelope.total
  } else {
    throw new Error('Invalid channels page response')
  }
  return { items: envelope.items as ChannelRow[], total }
}

/** Parse the fleet-wide error summary response. */
export function parseChannelsErrorSummary(
  result: unknown
): ChannelsErrorSummary {
  if (!result || typeof result !== 'object') {
    throw new Error('Invalid channels error summary response')
  }
  const envelope = result as {
    total?: unknown
    errorCount?: unknown
    byStatus?: unknown
  }
  if (
    typeof envelope.total !== 'number' ||
    typeof envelope.errorCount !== 'number' ||
    !envelope.byStatus ||
    typeof envelope.byStatus !== 'object'
  ) {
    throw new Error('Invalid channels error summary response')
  }
  return {
    total: envelope.total,
    errorCount: envelope.errorCount,
    byStatus: envelope.byStatus as ChannelsErrorSummary['byStatus'],
  }
}

/** Shared queryFn for the legacy full channels list (one-shot drilldown). */
async function getChannelsList(): Promise<ChannelRow[]> {
  const result = await api.getChannels()
  return parseChannelsEnvelope(result)
}

export function useChannels(
  queryOptions?: Omit<
    UseQueryOptions<ChannelRow[], Error, ChannelRow[]>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery<ChannelRow[], Error>({
    queryKey: channelsKeys.list(),
    queryFn: getChannelsList,
    staleTime: 10 * 1000,
    ...queryOptions,
  })
}

/** Fetch one server-side channels page by URL-owned table state. */
export async function fetchChannelsPage(params: {
  pageIndex: number
  pageSize: number
  status?: string
}): Promise<ChannelsPageData> {
  const result = await api.getChannels({
    page: params.pageIndex + 1,
    pageSize: params.pageSize,
    status: params.status || undefined,
  })
  return parseChannelsPageEnvelope(result)
}

export function useChannelsPage(
  params: { pageIndex: number; pageSize: number; status?: string },
  options?: Omit<UseQueryOptions<ChannelsPageData>, 'queryKey' | 'queryFn'>
) {
  return useQuery<ChannelsPageData>({
    queryKey: channelsKeys.page(
      params.pageIndex,
      params.pageSize,
      params.status
    ),
    queryFn: () => fetchChannelsPage(params),
    placeholderData: (previous) => previous,
    staleTime: 10 * 1000,
    ...options,
  })
}

/** Fetch the fleet-wide channel error summary. */
export async function getChannelsErrorSummary(): Promise<ChannelsErrorSummary> {
  return parseChannelsErrorSummary(await api.getChannelsErrorSummary())
}

export function useChannelsErrorSummary(
  options?: Omit<UseQueryOptions<ChannelsErrorSummary>, 'queryKey' | 'queryFn'>
) {
  return useQuery<ChannelsErrorSummary>({
    queryKey: channelsKeys.errorSummary(),
    queryFn: getChannelsErrorSummary,
    staleTime: 10 * 1000,
    ...options,
  })
}

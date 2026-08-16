// metapi-go/features/channels — TanStack Query hook wrapping `lib/api.ts`.
// GET /api/channels returns a paginated envelope
// `{items: ChannelRow[], total, page, pageSize}` (see handler/admin/channels.go),
// so `useChannels` parses the envelope and surfaces only the `items` array as
// the read-only list. The table still filters/sorts/pages client-side.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import { channelsKeys, type ChannelRow } from './types'

/**
 * Parse the GET /api/channels envelope. Returns the `items` array when the
 * body is a well-formed `{items: [...]}` object; throws otherwise so a
 * malformed body fails the query explicitly instead of masquerading as an
 * empty list.
 */
export function parseChannelsEnvelope(result: unknown): ChannelRow[] {
  if (result && typeof result === 'object') {
    const items = (result as { items?: unknown }).items
    if (Array.isArray(items)) return items as ChannelRow[]
  }
  throw new Error('Invalid channels response')
}

/** Shared queryFn for the channels list (route loader + useChannels). */
export async function getChannelsList(): Promise<ChannelRow[]> {
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

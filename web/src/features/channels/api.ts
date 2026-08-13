// metapi-go/features/channels — TanStack Query hook wrapping `lib/api.ts`.
// GET /api/channels returns the full read-only list, so `useChannels` is a
// single query and the table does client-side filtering/sorting/pagination.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import { channelsKeys, type ChannelRow } from './types'

export function useChannels(
  queryOptions?: Omit<
    UseQueryOptions<ChannelRow[], Error, ChannelRow[]>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery<ChannelRow[], Error>({
    queryKey: channelsKeys.list(),
    queryFn: async () => {
      const result = await api.getChannels()
      return (result ?? []) as ChannelRow[]
    },
    staleTime: 10 * 1000,
    ...queryOptions,
  })
}

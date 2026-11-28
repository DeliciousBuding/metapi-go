import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { channelsKeys } from '@/features/channels'
import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import { routeQueryKeys } from './route-query-keys'
import type { RouteChannel } from './types'

export function useRouteChannels(
  routeId: number | null,
  options?: Omit<
    UseQueryOptions<RouteChannel[]>,
    'queryKey' | 'queryFn' | 'enabled'
  >
) {
  return useQuery({
    queryKey: routeQueryKeys.channels(routeId ?? 0),
    queryFn: async () => {
      const channels = await api.getRouteChannels(routeId as number)
      if (!Array.isArray(channels))
        throw new Error('Failed to load route channels')
      return channels as RouteChannel[]
    },
    enabled: routeId != null,
    staleTime: 15 * 1000,
    ...options,
  })
}

export function useClearRouteCooldown() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.clearRouteCooldown(id)
      return assertBusinessOk(result, 'tokenRoutes.toast.cooldownClearFailed')
    },
    onSuccess: (_data, routeId) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      // The route detail sheet renders cooldown badges from the per-route
      // channel list; invalidate it so the badges (and the gated clear
      // button) flip back to enabled immediately instead of after the
      // 15s staleTime.
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channels(routeId),
      })
      // Channels page status badges (`cooldown`/`breaker_open`) derive from
      // the same rows — refresh them too.
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.cooldownCleared'))
    },
  })
}

/**
 * Per-channel operator edits (weight / priority / enabled). The backend
 * marks `manual_override=true` on any edit so a route rebuild cannot wipe
 * operator tuning; the route detail sheet and channel detail sheet surface
 * the same fields, so the summary + both channel lists are invalidated.
 */

export function useUpdateChannel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number
      data: { weight?: number; priority?: number; enabled?: boolean }
    }) => {
      const result = await api.updateChannel(id, data)
      return assertBusinessOk(result, 'tokenRoutes.toast.channelUpdateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channelsAll(),
      })
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.channelUpdated'))
    },
  })
}

export function useDeleteChannel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteChannel(id)
      return assertBusinessOk(result, 'tokenRoutes.toast.channelDeleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channelsAll(),
      })
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.channelDeleted'))
    },
  })
}

export interface BatchAddChannelsResult {
  created?: number
  skipped?: number
  errors?: string[]
}

export function useBatchAddChannels() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      routeId,
      channels,
    }: {
      routeId: number
      channels: Array<{
        accountId: number
        tokenId?: number
        sourceModel?: string
      }>
    }) => {
      const result = await api.batchAddChannels(routeId, channels)
      return assertBusinessOk<BatchAddChannelsResult>(
        result,
        'tokenRoutes.toast.channelAddFailed'
      )
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channels(variables.routeId),
      })
    },
  })
}

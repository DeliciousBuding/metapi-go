// metapi-go/routes — channels read-only list.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { channelsKeys, channelsSearchSchema } from '@/features/channels'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/channels')({
  validateSearch: channelsSearchSchema,
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: channelsKeys.list(),
      queryFn: async () => {
        const result = await api.getChannels()
        return (result ?? []) as unknown[]
      },
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/channels/components/channels-page'),
    'ChannelsPage'
  ),
})

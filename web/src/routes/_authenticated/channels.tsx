// metapi-go/routes — channels read-only list.

import { createFileRoute } from '@tanstack/react-router'

import { channelsKeys, channelsSearchSchema, getChannelsList } from '@/features/channels'
import { ChannelsPage } from '@/features/channels/components/channels-page'

export const Route = createFileRoute('/_authenticated/channels')({
  validateSearch: channelsSearchSchema,
  staticData: { title: 'channels.page.title' },
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: channelsKeys.list(),
      queryFn: () => getChannelsList(),
    })
  },
  component: ChannelsPage,
})

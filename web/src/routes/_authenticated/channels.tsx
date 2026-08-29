// metapi-go/routes — channels read-only list.
//
// The loader prefetches ONLY the server-paginated page pointed at by the URL
// plus the independent fleet-wide error summary. The legacy full-list query
// remains available lazily for one-shot `?channelId=` drilldown.

import { createFileRoute } from '@tanstack/react-router'

import {
  channelsKeys,
  channelsSearchSchema,
  fetchChannelsPage,
  getChannelsErrorSummary,
} from '@/features/channels'
import { ChannelsPage } from '@/features/channels/components/channels-page'
import { asStringParam } from '@/lib/helpers/searchParams'

export const Route = createFileRoute('/_authenticated/channels')({
  validateSearch: channelsSearchSchema,
  staticData: { title: 'channels.page.title' },
  loader: async ({ context, location }) => {
    const params = new URLSearchParams(location.searchStr)
    const raw = {
      page: params.get('page') ?? undefined,
      pageSize: params.get('pageSize') ?? undefined,
      status: params.get('status') ?? undefined,
    }
    const parsed = channelsSearchSchema.safeParse(raw)
    const pageIndex = parsed.success ? (parsed.data.page ?? 0) : 0
    const pageSize = parsed.success ? (parsed.data.pageSize ?? 20) : 20
    const status = parsed.success
      ? asStringParam(parsed.data.status) || undefined
      : undefined

    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: channelsKeys.page(pageIndex, pageSize, status),
        queryFn: () => fetchChannelsPage({ pageIndex, pageSize, status }),
      }),
      context.queryClient.prefetchQuery({
        queryKey: channelsKeys.errorSummary(),
        queryFn: getChannelsErrorSummary,
      }),
    ])
  },
  component: ChannelsPage,
})

type ComparisonChannel = {
  id: number
  enabled: boolean
}

export function filterEnabledComparisonChannels<
  Channel extends ComparisonChannel,
>(channels: readonly Channel[]): Channel[] {
  return channels.filter((channel) => channel.enabled)
}

export function retainEnabledComparisonChannelIds(
  selectedChannelIds: readonly number[],
  channels: readonly ComparisonChannel[]
): number[] {
  const enabledChannelIds = new Set(
    channels.filter((channel) => channel.enabled).map((channel) => channel.id)
  )
  return selectedChannelIds.filter((channelId) =>
    enabledChannelIds.has(channelId)
  )
}

/**
 * Human-readable label for a channel row in the tester pickers. Several
 * routes can share the same account+site (one channel per model), which
 * makes `name · site` labels identical for different choices, so the label
 * carries the model list and the channel id to keep every row
 * distinguishable.
 */
export function formatChannelLabel(channel: {
  id: number
  name: string
  site: { name: string }
  models?: string
}): string {
  const model = channel.models?.trim() ? ' · ' + channel.models.trim() : ''
  return channel.name + model + ' · ' + channel.site.name + ' (#' + channel.id + ')'
}

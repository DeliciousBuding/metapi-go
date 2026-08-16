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

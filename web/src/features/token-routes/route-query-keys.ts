export const routeQueryKeys = {
  all: ['routes'] as const,
  summary: () => [...routeQueryKeys.all, 'summary'] as const,
  rebuild: () => [...routeQueryKeys.all, 'rebuild'] as const,
  candidates: () => [...routeQueryKeys.all, 'candidates'] as const,
  channels: (id: number) => ['routes', 'channels', id] as const,
  channelsAll: () => ['routes', 'channels'] as const,
  decision: (input: { model: string; endpoint: string } | null) =>
    [...routeQueryKeys.all, 'decision', input] as const,
}

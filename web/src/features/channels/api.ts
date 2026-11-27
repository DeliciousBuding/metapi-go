// metapi-go/features/channels — TanStack Query hooks wrapping `lib/api.ts`.
// The channels page is server-paginated (`GET /api/channels?page=&pageSize=`)
// and the error banner uses the independent fleet-wide error-summary endpoint
// so the count remains truthful even when only one page is loaded.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { request } from '@/lib/api/transport'

import {
  channelsKeys,
  type ChannelRow,
  type ChannelsErrorSummary,
  type ChannelsPageData,
} from './types'

export type ChannelProbeMode = 'models' | 'chat'

export type ChannelProbeRequest = {
  channelId: number
  mode: ChannelProbeMode
  model?: string
}

/**
 * Response returned by `POST /api/admin/test-channel`.
 *
 * `firstByteLatencyMs` and `upstreamReportedModel` are optional additive fields
 * so older servers that omit them remain fully supported without fabrication.
 */
export type ChannelProbeResult = {
  success: boolean
  statusCode: number
  latencyMs: number
  firstByteLatencyMs?: number | null
  upstreamReportedModel?: string | null
  truncatedBody: string
  error: string | null
  channelId?: number
  oauthRouteUnitId?: number | null
  routeId?: number
  siteId?: number
  siteName?: string
  accountId?: number
  model?: string
  mode?: ChannelProbeMode | string
  upstreamPath?: string
  upstreamHost?: string
  bodyTruncated?: boolean
}

function readOptionalFiniteNumber(value: unknown): number | null | undefined {
  if (value === undefined) return undefined
  if (value === null) return null
  if (typeof value === 'number' && Number.isFinite(value)) return value
  return undefined
}

function readOptionalTrimmedString(value: unknown): string | null | undefined {
  if (value === undefined) return undefined
  if (value === null) return null
  if (typeof value === 'string') {
    const trimmed = value.trim()
    return trimmed ? trimmed : null
  }
  return undefined
}

/** Parse and validate the `POST /api/admin/test-channel` response payload. */
export function parseChannelProbeResult(result: unknown): ChannelProbeResult {
  if (!result || typeof result !== 'object') {
    throw new Error('Invalid channel probe response')
  }
  const body = result as Record<string, unknown>
  if (
    typeof body.success !== 'boolean' ||
    typeof body.statusCode !== 'number' ||
    !Number.isFinite(body.statusCode) ||
    typeof body.latencyMs !== 'number' ||
    !Number.isFinite(body.latencyMs)
  ) {
    throw new Error('Invalid channel probe response')
  }
  return {
    success: body.success,
    statusCode: body.statusCode,
    latencyMs: body.latencyMs,
    firstByteLatencyMs: readOptionalFiniteNumber(body.firstByteLatencyMs),
    upstreamReportedModel: readOptionalTrimmedString(
      body.upstreamReportedModel
    ),
    truncatedBody:
      typeof body.truncatedBody === 'string' ? body.truncatedBody : '',
    error: typeof body.error === 'string' && body.error ? body.error : null,
    channelId: readOptionalFiniteNumber(body.channelId) ?? undefined,
    oauthRouteUnitId: readOptionalFiniteNumber(body.oauthRouteUnitId),
    routeId: readOptionalFiniteNumber(body.routeId) ?? undefined,
    siteId: readOptionalFiniteNumber(body.siteId) ?? undefined,
    siteName: typeof body.siteName === 'string' ? body.siteName : undefined,
    accountId: readOptionalFiniteNumber(body.accountId) ?? undefined,
    model: typeof body.model === 'string' ? body.model : undefined,
    mode: typeof body.mode === 'string' ? body.mode : undefined,
    upstreamPath:
      typeof body.upstreamPath === 'string' ? body.upstreamPath : undefined,
    upstreamHost:
      typeof body.upstreamHost === 'string' ? body.upstreamHost : undefined,
    bodyTruncated:
      typeof body.bodyTruncated === 'boolean' ? body.bodyTruncated : undefined,
  }
}

/**
 * Run a synchronous single-channel diagnostic probe via
 * `POST /api/admin/test-channel`. Business errors and HTTP errors are surfaced
 * inline by the probe card rather than triggering duplicate global toasts.
 */
export async function probeChannel(
  input: ChannelProbeRequest,
  signal?: AbortSignal
): Promise<ChannelProbeResult> {
  const trimmedModel = input.model?.trim()
  const payload: {
    channelId: number
    mode: ChannelProbeMode
    model?: string
  } = {
    channelId: input.channelId,
    mode: input.mode,
    ...(trimmedModel ? { model: trimmedModel } : {}),
  }
  const response = await request<unknown>('/api/admin/test-channel', {
    method: 'POST',
    body: JSON.stringify(payload),
    signal,
    timeoutMs: 65_000,
    skipBusinessError: true,
    skipErrorHandler: true,
  })
  return parseChannelProbeResult(response)
}

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

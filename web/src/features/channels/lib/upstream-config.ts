import { t } from 'i18next'

import type {
  ImportedCredential,
  ImportedUpstreamDetail,
  ImportedUpstreamUpdate,
} from '@/lib/api/imported-upstreams'
import { parseStringMap, serializeStringMap } from '@/lib/helpers/string-map'

import type { ConnectionValues } from './upstream-connection-schema'

export {
  connectionSchema,
  providerEndpointError,
  type ConnectionValues,
} from './upstream-connection-schema'
export { modelWireUrlSchema } from './upstream-endpoint-schema'
export {
  availableProfiles,
  memberProtocols,
  upstreamProtocolGroups,
  upstreamProtocols,
} from './upstream-protocols'

export function credentialKindLabel(kind: ImportedCredential['kind']) {
  if (kind === 'none') return t('channels.upstream.noAuthentication')
  if (kind === 'gcp_service_account') {
    return t('channels.upstream.gcpServiceAccount')
  }
  return kind === 'oauth' ? 'OAuth' : 'API Key'
}

export const upstreamKeys = {
  all: ['imported-upstreams'] as const,
  detail: (id: number) => ['imported-upstreams', id] as const,
  credentials: (id: number) =>
    ['imported-upstreams', id, 'credentials'] as const,
  models: (id: number) => ['imported-upstreams', id, 'models'] as const,
  groups: ['imported-upstreams', 'groups'] as const,
}

export function connectionValues(
  detail: ImportedUpstreamDetail
): ConnectionValues {
  return {
    pluginBindings: [],
    providerBinding: null,
    streamPolicy: detail.streamPolicy ?? 'unlimited',
    rateLimit: {
      rpm: detail.rateLimit?.rpm ?? 0,
      tpm: detail.rateLimit?.tpm ?? 0,
      maxConcurrent: detail.rateLimit?.maxConcurrent ?? 0,
      queueSize: detail.rateLimit?.queueSize ?? 0,
      queueTimeoutMs: detail.rateLimit?.queueTimeoutMs ?? 0,
    },
    name: detail.name,
    baseUrl: detail.baseUrl,
    endpointConfig: detail.endpointConfig ?? {},
    useSystemProxy: detail.useSystemProxy,
    openaiChatCompletionPath: detail.openaiChatCompletionPath ?? '',
    openaiResponsePath: detail.openaiResponsePath ?? '',
    anthropicMessagePath: detail.anthropicMessagePath ?? '',
    channelProxy: '',
    customHeaders: '',
    paramOverride: '',
  }
}

// Ordered header rows use the shared string-map editor without collapsing
// duplicate keys or changing the server's array contract.
export function headersForEditor(raw: string): string {
  if (!raw.trim()) return ''
  const rows: Array<{ header_key: string; header_value: string }> =
    JSON.parse(raw)
  return serializeStringMap(
    rows.map((row) => ({ key: row.header_key, value: row.header_value }))
  )
}

export function headersForRequest(value: string): string {
  const entries = parseStringMap(value)
  if (!entries) throw new Error('Invalid header rows')
  return entries.length
    ? JSON.stringify(
        entries.map((row) => ({ header_key: row.key, header_value: row.value }))
      )
    : ''
}

export function connectionPatch(
  values: ConnectionValues,
  dirty: Partial<Record<keyof ConnectionValues, unknown>>
): ImportedUpstreamUpdate {
  const patch: ImportedUpstreamUpdate = {}
  for (const key of Object.keys(dirty) as Array<keyof ConnectionValues>) {
    if (!dirty[key]) continue
    if (key === 'rateLimit') {
      // The API replaces the complete policy, not individual dimensions.
      // Retain every nonzero sibling; all zeroes explicitly clear with {}.
      patch.rateLimit = Object.fromEntries(
        Object.entries(values.rateLimit).filter(([, value]) => value > 0)
      )
      continue
    }
    Object.assign(patch, {
      [key]:
        key === 'customHeaders'
          ? headersForRequest(values.customHeaders)
          : values[key],
    })
  }
  return patch
}

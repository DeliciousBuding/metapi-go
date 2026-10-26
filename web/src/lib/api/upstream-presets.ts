import type { ImportedEndpointConfig } from './imported-upstreams'
import { request } from './transport'

export type UpstreamPreset = {
  id: string
  name: string
  label: string
  provider: string
  platform: string
  group: 'domestic' | 'coding' | 'gateway' | 'other'
  defaultUrl: string
  requiresBaseUrl: boolean
  credentialMode: 'apiKey' | 'optional' | 'oauth' | 'gcpServiceAccount'
  /** Resolving parameterized endpoint templates (Vertex) needs projectId. */
  requiresProject?: boolean
  /** Template defaults; custom endpoint configuration may add other protocols. */
  protocols: Array<keyof ImportedEndpointConfig>
  recommendedModels: string[]
}

export const upstreamPresetsApi = {
  getUpstreamPresets: () =>
    request<{ items: UpstreamPreset[] }>('/api/imported-upstreams/presets'),
  resolveUpstreamPreset: (
    input: { presetId: string; baseUrl: string; projectId?: string },
    signal?: AbortSignal
  ) =>
    request<{
      presetId: string
      provider: string
      baseUrl: string
      endpointConfig: ImportedEndpointConfig
    }>('/api/imported-upstreams/presets/resolve', {
      method: 'POST',
      body: JSON.stringify(input),
      signal,
      skipErrorHandler: true,
    }),
}

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
  protocols: Array<'chat' | 'responses' | 'messages' | 'gemini'>
  recommendedModels: string[]
}

export type ResolvedUpstreamPreset = {
  provider: string
  endpointConfig: ImportedEndpointConfig
}

export const upstreamPresetsApi = {
  getUpstreamPresets: () =>
    request<{ items: UpstreamPreset[] }>('/api/imported-upstreams/presets'),
  resolveUpstreamPreset: (input: { presetId: string; baseUrl: string }) =>
    request<ResolvedUpstreamPreset>('/api/imported-upstreams/presets/resolve', {
      method: 'POST',
      body: JSON.stringify(input),
      skipErrorHandler: true,
    }),
}

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
  credentialMode: 'apiKey' | 'optional' | 'oauth'
  protocols: Array<keyof ImportedEndpointConfig>
  recommendedModels: string[]
}

export const upstreamPresetsApi = {
  getUpstreamPresets: () =>
    request<{ items: UpstreamPreset[] }>('/api/imported-upstreams/presets'),
}

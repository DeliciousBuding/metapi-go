import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import {
  upstreamPresetsApi,
  type UpstreamPreset,
} from '@/lib/api/upstream-presets'

import { connectionSchema } from './upstream-config'

export async function resolvePresetEndpoints(
  preset: UpstreamPreset,
  input: { baseUrl: string; projectId?: string },
  signal: AbortSignal
) {
  const result = await upstreamPresetsApi.resolveUpstreamPreset(
    { presetId: preset.id, ...input },
    signal
  )
  if (
    result.presetId !== preset.id ||
    result.provider !== preset.provider ||
    !connectionSchema.shape.endpointConfig.safeParse(result.endpointConfig)
      .success ||
    !Object.values(result.endpointConfig).some(Boolean) ||
    preset.protocols.some((key) => !result.endpointConfig[key]) ||
    Object.keys(result.endpointConfig).some(
      (key) => !preset.protocols.includes(key as keyof ImportedEndpointConfig)
    )
  ) {
    throw new Error('Invalid resolved upstream preset')
  }
  return result.endpointConfig
}

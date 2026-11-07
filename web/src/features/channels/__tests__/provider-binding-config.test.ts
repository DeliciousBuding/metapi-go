import { describe, expect, it } from 'vitest'

import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import {
  connectionPatch,
  connectionSchema,
  connectionValues,
} from '../lib/upstream-config'
import {
  upstreamCreateDefaults,
  upstreamCreatePayload,
  upstreamCreateSchema,
} from '../lib/upstream-create'

const binding = {
  key: 'fixture-driver',
  protocols: ['chat'] as const,
  options: { mode: 'safe' },
  catalog: { url: 'https://fixture.invalid/catalog', auth: 'bearer' as const },
}
const detail: ImportedUpstreamDetail = {
  id: 1,
  name: 'Fixture',
  originKey: '',
  provider: 'custom',
  dialect: 'generic',
  enabled: true,
  baseUrl: 'https://fixture.invalid',
  endpointConfig: {
    chat: { url: 'https://fixture.invalid/chat', auth: 'bearer' },
  },
  openaiChatCompletionPath: '',
  openaiResponsePath: '',
  anthropicMessagePath: '',
  useSystemProxy: false,
  hasChannelProxy: false,
  hasCustomHeaders: false,
  hasParamOverride: false,
  channelProxyDisplay: '',
}
const preset: UpstreamPreset = {
  id: 'fixture',
  name: 'Fixture',
  label: 'Fixture',
  platform: 'openai',
  provider: 'custom',
  group: 'gateway',
  defaultUrl: detail.baseUrl,
  protocols: ['chat'],
  recommendedModels: [],
  requiresBaseUrl: false,
  credentialMode: 'apiKey',
}

describe('provider binding form contract', () => {
  it('preserves a loaded provider and only writes or clears it when explicitly edited', () => {
    const values = connectionSchema.parse({
      ...connectionValues(detail),
      providerBinding: binding,
    })
    expect(connectionPatch(values, { providerBinding: true })).toEqual({
      providerBinding: binding,
    })
    expect(connectionPatch(values, { name: true })).toEqual({ name: 'Fixture' })
    const cleared = connectionSchema.parse({ ...values, providerBinding: null })
    expect(connectionPatch(cleared, { providerBinding: true })).toEqual({
      providerBinding: null,
    })
  })
  it('passes the independent provider through quick connect without a wire binding or whole transform', () => {
    const values = upstreamCreateSchema(preset).parse({
      ...upstreamCreateDefaults,
      presetId: preset.id,
      protocols: ['chat'],
      apiKey: 'fixture-key',
      baseUrl: detail.baseUrl,
      providerBinding: binding,
    })
    expect(upstreamCreatePayload(values, preset)).toEqual({
      presetId: preset.id,
      protocols: ['chat'],
      apiKey: 'fixture-key',
      providerBinding: binding,
    })
  })
})

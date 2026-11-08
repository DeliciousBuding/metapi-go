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
  it('binds each image query to its own endpoint on edit and custom quick connect', () => {
    const imageDetail: ImportedUpstreamDetail = {
      ...detail,
      endpointConfig: {
        imageGeneration: {
          url: 'https://images.invalid/generate',
          auth: 'bearer',
        },
        imageEdit: { url: 'https://edits.invalid/edit', auth: 'bearer' },
      },
    }
    const images = {
      key: 'fixture-driver',
      protocols: ['imageGeneration', 'imageEdit'],
      imageQueries: {
        imageGeneration: {
          url: 'https://images.invalid/tasks/{taskId}',
          auth: 'none',
        },
        imageEdit: {
          url: 'https://edits.invalid/tasks/{taskId}',
          auth: 'x-api-key',
        },
      },
    }
    const connection = {
      ...connectionValues(imageDetail),
      providerBinding: images,
    }
    const create = {
      ...upstreamCreateDefaults,
      presetId: preset.id,
      customEndpoints: true,
      endpointConfig: imageDetail.endpointConfig,
      apiKey: 'fixture-key',
      providerBinding: images,
    }
    expect(connectionSchema.safeParse(connection).success).toBe(true)
    expect(upstreamCreateSchema(preset).safeParse(create).success).toBe(true)
    for (const url of [
      'https://edits.invalid/tasks/{taskId}',
      'http://images.invalid/tasks/{taskId}',
      'https://images.invalid:444/tasks/{taskId}',
    ]) {
      const providerBinding = {
        ...images,
        imageQueries: {
          ...images.imageQueries,
          imageGeneration: { url, auth: 'none' },
        },
      }
      for (const result of [
        connectionSchema.safeParse({ ...connection, providerBinding }),
        upstreamCreateSchema(preset).safeParse({ ...create, providerBinding }),
      ]) {
        expect(result.success).toBe(false)
        if (!result.success)
          expect(result.error.issues).toContainEqual(
            expect.objectContaining({
              path: ['providerBinding'],
              message: 'channels.plugins.imageQueryOriginMismatch',
            })
          )
      }
    }
    const values = connectionSchema.parse(connection)
    expect(connectionPatch(values, { providerBinding: true })).toEqual({
      providerBinding: images,
    })
    expect(connectionPatch(values, { name: true })).toEqual({ name: 'Fixture' })
  })
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

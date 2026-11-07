import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  channelPluginsApi,
  pluginBindingSchema,
  providerBindingSchema,
} from '../channel-plugins'

const mock = vi.hoisted(() => vi.fn())
vi.mock('../transport', () => ({ request: mock }))
beforeEach(() => mock.mockReset())

describe('channel plugin API data boundary', () => {
  it('retains explicit anonymous provider metadata and catalog authentication', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'anonymous',
      name: 'Anonymous',
      version: '1.0.0',
      protocols: ['messages'],
      credentialKinds: ['none'],
      hooks: ['request', 'response', 'modelsRequest', 'modelsResponse'],
      defaults: {
        endpoints: {
          messages: {
            path: '/v1/messages',
            auth: 'none',
            profile: 'ollama-messages',
          },
        },
        catalog: { path: '/inventory', auth: 'none' },
      },
    }
    const value = {
      items: [{ key: 'anonymous', enabled: true, revision: 1, manifest }],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
    const binding = {
      key: 'anonymous',
      protocols: ['messages'],
      catalog: { url: 'http://localhost:11434/inventory', auth: 'none' },
    }
    expect(providerBindingSchema.parse(binding)).toEqual(binding)
    mock.mockResolvedValueOnce({
      items: [
        {
          ...value.items[0],
          manifest: { ...manifest, credentialKinds: ['none', 'none'] },
        },
      ],
    })
    await expect(channelPluginsApi.list()).rejects.toThrow()
  })
  it('reads decoder hooks and original observation selectors without granting new protocols', async () => {
    const manifest = {
      apiVersion: 2,
      kind: 'provider',
      key: 'decoder',
      name: 'Decoder',
      version: '1.0.0',
      protocols: ['chat'],
      credentialKinds: ['api_key'],
      hooks: ['request', 'response', 'event', 'decodeResponse', 'decodeEvent'],
      observations: {
        chat: {
          response: {
            usagePaths: { prompt_tokens: '/a.b/a~1b/~0count' },
            modelPath: '/engine',
          },
          event: {
            usagePaths: { completion_tokens: '/meter/output' },
            errorPath: '/failure',
          },
        },
      },
    }
    const item = (meta: unknown) => ({
      items: [{ key: 'decoder', enabled: true, revision: 1, manifest: meta }],
    })
    mock.mockResolvedValueOnce(item(manifest))
    await expect(channelPluginsApi.list()).resolves.toEqual(item(manifest))
    for (const invalid of [
      { ...manifest, observations: { responses: manifest.observations.chat } },
      {
        ...manifest,
        observations: {
          chat: { response: { usagePaths: { invoice: '/amount' } } },
        },
      },
      {
        ...manifest,
        observations: {
          chat: {
            response: { usagePaths: { prompt_tokens: '/bad~2pointer' } },
          },
        },
      },
      { ...manifest, observations: { chat: {} } },
      { ...manifest, observations: { chat: { response: { usagePaths: {} } } } },
    ]) {
      mock.mockResolvedValueOnce(item(invalid))
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('keeps provider manifests distinct and preserves their configured defaults', async () => {
    const value = {
      items: [
        {
          key: 'driver',
          revision: 1,
          enabled: true,
          manifest: {
            apiVersion: 2,
            kind: 'provider',
            key: 'driver',
            name: 'Driver',
            version: '1.0.0',
            protocols: ['chat'],
            credentialKinds: ['api_key'],
            hooks: ['request', 'response', 'modelsRequest', 'modelsResponse'],
            defaults: {
              endpoints: { chat: { path: '/generate', auth: 'bearer' } },
              catalog: {
                path: '/models',
                auth: 'bearer',
                cursorParameter: 'cursor',
              },
            },
          },
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
  })
  it('keeps compact and WebSocket as separate explicit binding slots', async () => {
    const protocols = [
      'chat',
      'responses',
      'messages',
      'gemini',
      'responsesCompact',
      'responsesWebsocket',
    ]
    const value = {
      items: [
        {
          key: 'native',
          revision: 1,
          enabled: true,
          manifest: {
            apiVersion: 1,
            key: 'native',
            name: 'Native',
            version: '1.0.0',
            protocols,
            hooks: ['request', 'event'],
          },
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
    expect(pluginBindingSchema.parse({ key: 'native', protocols })).toEqual({
      key: 'native',
      protocols,
    })
    for (const protocol of ['responsesCompact', 'responsesWebsocket']) {
      expect(
        pluginBindingSchema.parse({ key: 'native', protocols: [protocol] })
          .protocols
      ).toEqual([protocol])
    }
    expect(() =>
      pluginBindingSchema.parse({
        key: 'native',
        protocols: ['futureTransport'],
      })
    ).toThrow()
  })
  it('rejects malformed snapshots rather than accepting an empty list', async () => {
    for (const value of [
      {},
      { items: null },
      { items: [{ key: 'fixture', enabled: true, revision: 0 }] },
      { items: [{ key: 'fixture', enabled: true, revision: 1 }] },
      {
        items: [
          {
            key: 'fixture',
            enabled: true,
            revision: 1,
            manifest: {
              apiVersion: 1,
              key: 'other',
              name: 'Other',
              version: '1',
              protocols: ['chat'],
              hooks: ['request'],
            },
          },
        ],
      },
    ]) {
      mock.mockResolvedValueOnce(value)
      await expect(channelPluginsApi.list()).rejects.toThrow()
    }
  })
  it('keeps a damaged item available for management recovery', async () => {
    const value = {
      items: [
        {
          key: 'fixture',
          enabled: false,
          revision: 1,
          loadError: 'channel_plugin_load_failed',
        },
      ],
    }
    mock.mockResolvedValueOnce(value)
    await expect(channelPluginsApi.list()).resolves.toEqual(value)
  })
  it('rejects a wrong identity or revision on reads and writes', async () => {
    mock.mockResolvedValueOnce({
      key: 'other',
      revision: 4,
      enabled: true,
      source: 'synthetic',
    })
    await expect(channelPluginsApi.detail('fixture')).rejects.toThrow()
    for (const response of [
      {},
      { key: 'other', revision: 2, enabled: true },
      { key: 'fixture', revision: 1, enabled: true },
      { key: 'fixture', revision: 2, enabled: false },
    ]) {
      mock.mockResolvedValueOnce(response)
      await expect(
        channelPluginsApi.save('fixture', {
          source: 'synthetic',
          enabled: true,
          expectedRevision: 1,
        })
      ).rejects.toThrow()
    }
  })
})

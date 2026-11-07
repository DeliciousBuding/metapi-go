import { beforeEach, describe, expect, it, vi } from 'vitest'

import { channelPluginsApi, pluginBindingSchema } from '../channel-plugins'

const mock = vi.hoisted(() => vi.fn())
vi.mock('../transport', () => ({ request: mock }))
beforeEach(() => mock.mockReset())

describe('channel plugin API data boundary', () => {
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

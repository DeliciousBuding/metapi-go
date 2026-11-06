import { beforeEach, describe, expect, it, vi } from 'vitest'

import { channelPluginsApi } from '../channel-plugins'

const mock = vi.hoisted(() => vi.fn())
vi.mock('../transport', () => ({ request: mock }))
beforeEach(() => mock.mockReset())

describe('channel plugin API data boundary', () => {
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

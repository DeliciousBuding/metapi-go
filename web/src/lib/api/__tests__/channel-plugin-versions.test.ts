import { beforeEach, describe, expect, it, vi } from 'vitest'

import { channelPluginVersionsApi } from '../channel-plugin-versions'

const mock = vi.hoisted(() => vi.fn())
vi.mock('../transport', () => ({ request: mock }))
beforeEach(() => mock.mockReset())
const version = {
  key: 'recovery',
  fingerprint: 'a'.repeat(64),
  available: false,
  taskReferences: 2,
}

describe('historical task program data boundary', () => {
  it('retains missing program references and rejects unreadable or private lists', async () => {
    mock.mockResolvedValueOnce({ items: [version] })
    await expect(channelPluginVersionsApi.list()).resolves.toEqual({
      items: [version],
    })
    for (const invalid of [
      null,
      {},
      { items: [version, version] },
      { items: [{ ...version, fingerprint: 'wrong' }] },
      { items: [{ ...version, taskReferences: -1 }] },
      { items: [{ ...version, taskReferences: 1.5 }] },
      { items: [{ ...version, source: 'private-source' }] },
    ]) {
      mock.mockResolvedValueOnce(invalid)
      await expect(channelPluginVersionsApi.list()).rejects.toThrow()
    }
  })

  it('sends the selected exact identity and refuses success for another program', async () => {
    const result = {
      key: version.key,
      fingerprint: version.fingerprint,
      restored: true,
    }
    mock.mockResolvedValueOnce(result)
    await expect(
      channelPluginVersionsApi.restore(version, 'reviewed-source')
    ).resolves.toEqual(result)
    expect(mock).toHaveBeenLastCalledWith(
      '/api/channel-plugin-versions/restore',
      {
        method: 'POST',
        body: JSON.stringify({
          key: version.key,
          fingerprint: version.fingerprint,
          source: 'reviewed-source',
        }),
      }
    )
    for (const invalid of [
      { ...result, key: 'another' },
      { ...result, fingerprint: 'b'.repeat(64) },
      { ...result, restored: false },
    ]) {
      mock.mockResolvedValueOnce(invalid)
      await expect(
        channelPluginVersionsApi.restore(version, 'reviewed-source')
      ).rejects.toThrow()
    }
  })
})

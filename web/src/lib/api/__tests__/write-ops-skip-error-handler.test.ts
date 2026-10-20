// Behavior test: the write operations whose callers surface their own
// specific error toasts must opt out of the global http-client error toast
// via `skipErrorHandler` — otherwise a failed request toasts twice (the
// generic interceptor message plus the caller's specific one). This locks
// the api-layer flag for every mutation listed in issue #889.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import { eventsApi } from '../events'
import { oauthApi } from '../oauth'
import { settingsApi } from '../settings'
import { statsApi } from '../stats'

const apiClientStub = vi.hoisted(() => ({
  get: vi.fn(),
  request: vi.fn(),
}))

vi.mock('@/lib/http-client', () => ({
  apiClient: apiClientStub,
  fetchAuthenticatedResponse: vi.fn(),
  extractResponseErrorMessage: vi.fn(),
}))

beforeEach(() => {
  apiClientStub.get.mockReset()
  apiClientStub.request.mockReset()
  apiClientStub.get.mockResolvedValue({ data: {} })
  apiClientStub.request.mockResolvedValue({ data: {} })
})

function lastRequestConfig(): Record<string, unknown> {
  const lastCall = apiClientStub.request.mock.calls.at(-1)
  if (!lastCall) throw new Error('apiClient.request was not called')
  // apiClient.request takes a single AxiosRequestConfig object.
  return (lastCall[0] ?? {}) as Record<string, unknown>
}

describe('caller-toasted write ops skip the global error toast', () => {
  it.each([
    { source: 'Octopus', replaceHeader: 'X-Octopus-Replace-Origin' },
    { source: 'AxonHub', replaceHeader: 'X-AxonHub-Replace-Origin' },
    { source: 'Magpie', replaceHeader: 'X-Magpie-Replace-Origin' },
  ])(
    'sends the reviewed replacement revision for $source',
    async ({ replaceHeader }) => {
      const payload =
        replaceHeader === 'X-Octopus-Replace-Origin'
          ? { version: 5, exported_at: '2026-10-06T00:00:00Z' }
          : replaceHeader === 'X-AxonHub-Replace-Origin'
            ? {
                version: '1.4',
                timestamp: '2026-10-06T00:00:00Z',
                channels: [],
                models: [],
              }
            : {
                format: 'magpie-backup',
                version: 1,
                kdf: 'pbkdf2-sha256',
                salt: 'c2FsdA==',
                nonce: 'bm9uY2U=',
                data: 'Y2lwaGVydGV4dA==',
              }
      const magpie = replaceHeader === 'X-Magpie-Replace-Origin'
      await settingsApi.importBackup(payload, {
        externalOriginKey: 'source',
        replacementRevision: 'reviewed-revision',
        ...(replaceHeader === 'X-Octopus-Replace-Origin'
          ? { replaceOctopusOrigin: true }
          : replaceHeader === 'X-AxonHub-Replace-Origin'
            ? { replaceAxonHubOrigin: true }
            : { replaceMagpieOrigin: true }),
        ...(magpie ? { magpiePassphrase: 'fixture-passphrase' } : {}),
      })
      expect(lastRequestConfig()).toMatchObject({
        url: '/api/settings/backup/import',
        data: magpie
          ? JSON.stringify({ data: payload, passphrase: 'fixture-passphrase' })
          : JSON.stringify(payload),
        headers: {
          'X-External-Origin-Key': 'source',
          'X-External-Replacement-Revision': 'reviewed-revision',
          [replaceHeader]: 'true',
        },
        skipErrorHandler: true,
      })
      expect(lastRequestConfig().headers).not.toHaveProperty(
        [
          'X-Octopus-Replace-Origin',
          'X-AxonHub-Replace-Origin',
          'X-Magpie-Replace-Origin',
        ].find((header) => header !== replaceHeader)!
      )
    }
  )

  it('sends the same external origin key on Octopus preview and commit', async () => {
    const payload = { version: 5, exported_at: '2026-10-06T00:00:00Z' }
    await settingsApi.previewBackupImport(payload, {
      externalOriginKey: 'octopus-lab',
    })
    expect(lastRequestConfig()).toMatchObject({
      url: '/api/settings/backup/import/preview',
      data: JSON.stringify(payload),
      headers: { 'X-External-Origin-Key': 'octopus-lab' },
      skipErrorHandler: true,
    })

    await settingsApi.importBackup(payload, {
      externalOriginKey: 'octopus-lab',
    })
    expect(lastRequestConfig()).toMatchObject({
      url: '/api/settings/backup/import',
      data: JSON.stringify(payload),
      headers: { 'X-External-Origin-Key': 'octopus-lab' },
      skipErrorHandler: true,
    })

    await settingsApi.importBackup(payload, {
      externalOriginKey: 'octopus-lab',
      octopusImportMode: 'channels-only',
    })
    expect(lastRequestConfig()).toMatchObject({
      url: '/api/settings/backup/import',
      data: JSON.stringify(payload),
      headers: {
        'X-External-Origin-Key': 'octopus-lab',
        'X-Octopus-Import-Mode': 'channels-only',
      },
      skipErrorHandler: true,
    })
  })

  it('oauth start / manual callback / quota refresh / rebind', async () => {
    await oauthApi.startOAuthProvider('openai', {})
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await oauthApi.submitOAuthManualCallback('state', 'https://cb')
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await oauthApi.refreshOAuthConnectionQuota(7)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await oauthApi.rebindOAuthConnection(7)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })
  })

  it('downstream key create / update / delete', async () => {
    await settingsApi.createDownstreamApiKey({ name: 'k' })
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await settingsApi.updateDownstreamApiKey(1, { name: 'k' })
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await settingsApi.deleteDownstreamApiKey(1)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })
  })

  it('announcement create / update / delete', async () => {
    const payload = { title: 't', message: 'm', severity: 'info' as const }

    await statsApi.createAnnouncement(payload)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.updateAnnouncement(1, payload)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.deleteAnnouncement(1)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.dismissAnnouncement(1)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })
  })

  it('model redirects generate / apply / promote / delete', async () => {
    await statsApi.generateModelRedirects(0)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.applyModelRedirects(false)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.updateModelRedirect(1, { source: 'manual' })
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await statsApi.deleteModelRedirect(1)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })
  })

  it('program events mark read / mark all / clear', async () => {
    await eventsApi.markEventRead(1)
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await eventsApi.markAllEventsRead()
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })

    await eventsApi.clearEvents()
    expect(lastRequestConfig()).toMatchObject({ skipErrorHandler: true })
  })
})

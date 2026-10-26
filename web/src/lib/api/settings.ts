import { detectExternalBackupSource } from '@/lib/helpers/external-backup-source'
import {
  fetchAuthenticatedResponse,
  extractResponseErrorMessage,
} from '@/lib/http-client'

import {
  parseCredentialExport,
  type CredentialExportProfileId,
} from './credential-export'
import { request } from './transport'
import type {
  BackupWebdavExportType,
  BackupWebdavResponse,
  SystemProxyTestRequest,
  RuntimeSettingsPayload,
  SettingsMigrationPreviewResponse,
  SettingsMigrationApplyResponse,
} from './types'

// Recognized external backups are posted verbatim; anything else is treated as a
// Metapi tables payload (which the backend also accepts inside a `data` wrapper).
function isExternalBackupPayload(data: unknown): boolean {
  return detectExternalBackupSource(data) !== null
}

export type BackupImportOptions = {
  externalOriginKey?: string
  octopusImportMode?: 'channels-only'
  replaceOctopusOrigin?: boolean
  replaceAxonHubOrigin?: boolean
  replaceMagpieOrigin?: boolean
  replacementRevision?: string
  magpiePassphrase?: string
  confirmToken?: string
}

function backupImportBody(data: unknown, magpiePassphrase?: string) {
  if (magpiePassphrase !== undefined) {
    return JSON.stringify({ data, passphrase: magpiePassphrase })
  }
  return JSON.stringify(isExternalBackupPayload(data) ? data : { data })
}

/**
 * Master-token re-confirmation header (#1034). Sensitive endpoints answer
 * `403 reauthRequired` until this header carries the current admin token; the
 * caller collects it from the ReauthDialog and replays the request once.
 */
function confirmHeader(
  confirmToken?: string
): Record<string, string> | undefined {
  return confirmToken ? { 'X-Admin-Confirm-Token': confirmToken } : undefined
}

export const settingsApi = {
  // Auth management
  getAuthInfo: () => request('/api/settings/auth/info'),
  // #1034: rotation is a sensitive op — the current master token is
  // re-presented in X-Admin-Confirm-Token (the form already collects it as
  // "old token", so no extra prompt is needed). Omitting newToken asks the
  // server to generate a strong random one; it is returned exactly once.
  changeAuthToken: (oldToken: string, newToken?: string) =>
    request<{ success: boolean; message?: string; newToken?: string }>(
      '/api/settings/auth/change',
      {
        method: 'POST',
        body: JSON.stringify(newToken ? { oldToken, newToken } : { oldToken }),
        headers: { 'X-Admin-Confirm-Token': oldToken },
        skipErrorHandler: true,
      }
    ),
  getRuntimeSettings: () => request('/api/settings/runtime'),
  getBrandList: () => request('/api/settings/brand-list'),
  updateRuntimeSettings: (
    data: RuntimeSettingsPayload,
    confirmToken?: string
  ) =>
    request('/api/settings/runtime', {
      method: 'PUT',
      body: JSON.stringify(data),
      headers: confirmHeader(confirmToken),
      skipErrorHandler: true,
    }),
  getUpdateCenterStatus: () => request('/api/update-center/status'),
  testSystemProxy: (data: SystemProxyTestRequest) =>
    request('/api/settings/system-proxy/test', {
      method: 'POST',
      body: JSON.stringify(data),
      timeoutMs: 20_000,
      skipErrorHandler: true,
    }),
  getRuntimeDatabaseConfig: () => request('/api/settings/database/runtime'),
  updateRuntimeDatabaseConfig: (
    data: Partial<{
      dialect: 'sqlite' | 'postgres'
      connectionString: string
      ssl: boolean
    }>
  ) =>
    request('/api/settings/database/runtime', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  testExternalDatabaseConnection: (
    data: {
      dialect: 'sqlite' | 'postgres'
      connectionString: string
      ssl?: boolean
    },
    confirmToken?: string
  ) =>
    request('/api/settings/database/test-connection', {
      method: 'POST',
      body: JSON.stringify(data),
      headers: confirmHeader(confirmToken),
      skipErrorHandler: true,
    }),
  /**
   * Queue a migration of the live runtime database onto an external target as
   * an admin background task. Resolves with the task ID once the backend has
   * accepted the job (202); progress is observed by polling `eventsApi.getTask`.
   */
  startDatabaseMigration: (
    data: {
      dialect: 'sqlite' | 'postgres'
      connectionString: string
      overwrite?: boolean
      ssl?: boolean
    },
    confirmToken?: string
  ) =>
    request<{ success: boolean; message: string; taskId: string }>(
      '/api/settings/database/migrate',
      {
        method: 'POST',
        body: JSON.stringify(data),
        headers: confirmHeader(confirmToken),
        skipErrorHandler: true,
      }
    ),
  getDownstreamApiKeys: () => request('/api/downstream-keys'),
  createDownstreamApiKey: (data: unknown, confirmToken?: string) =>
    request('/api/downstream-keys', {
      method: 'POST',
      body: JSON.stringify(data),
      headers: confirmHeader(confirmToken),
      // The keys section surfaces its own createFailed toast.
      skipErrorHandler: true,
    }),
  updateDownstreamApiKey: (id: number, data: unknown, confirmToken?: string) =>
    request(`/api/downstream-keys/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
      headers: confirmHeader(confirmToken),
      // The keys section surfaces its own updateFailed toast (form + toggle).
      skipErrorHandler: true,
    }),
  deleteDownstreamApiKey: (id: number) =>
    request(`/api/downstream-keys/${id}`, {
      method: 'DELETE',
      // The keys section surfaces its own deleteFailed toast.
      skipErrorHandler: true,
    }),
  // #1034: key export is a sensitive op; confirmToken carries the master
  // token for X-Admin-Confirm-Token re-confirmation.
  getDownstreamKeyExport: async (
    id: number,
    profile: 'all' | CredentialExportProfileId = 'all',
    confirmToken?: string,
    model?: string,
    signal?: AbortSignal
  ) => {
    const query = new URLSearchParams({ profile })
    if (model !== undefined) query.set('model', model)
    return parseCredentialExport(
      await request(`/api/downstream-keys/${id}/export?${query}`, {
        headers: confirmHeader(confirmToken),
        skipErrorHandler: true,
        disableDuplicate: true,
        signal,
      })
    )
  },
  /** Raw text export for file download; throws on non-OK responses. */
  exportBackupRaw: async (
    type: 'all' | 'accounts' | 'preferences' = 'all',
    confirmToken?: string
  ) => {
    const response = await fetchAuthenticatedResponse(
      `/api/settings/backup/export?type=${encodeURIComponent(type)}`,
      {
        headers: confirmToken
          ? { 'X-Admin-Confirm-Token': confirmToken }
          : undefined,
      }
    )
    if (!response.ok) {
      throw new Error(await extractResponseErrorMessage(response))
    }
    return response.text()
  },
  /** Semantic schedule migration preview (one-click upgrade). */
  getSettingsMigrationPreview: () =>
    request<SettingsMigrationPreviewResponse>(
      '/api/settings/migration/preview'
    ),
  /** Apply the semantic schedule migration (append-only v2 keys). */
  applySettingsMigration: () =>
    request<SettingsMigrationApplyResponse>('/api/settings/migration/apply', {
      method: 'POST',
      body: JSON.stringify({}),
      skipErrorHandler: true,
    }),
  importBackup: (data: unknown, options: BackupImportOptions = {}) =>
    request('/api/settings/backup/import', {
      method: 'POST',
      body: backupImportBody(data, options.magpiePassphrase),
      headers:
        options.externalOriginKey ||
        options.octopusImportMode ||
        options.replaceOctopusOrigin ||
        options.replaceAxonHubOrigin ||
        options.replaceMagpieOrigin ||
        options.replacementRevision ||
        options.confirmToken
          ? {
              ...(options.externalOriginKey
                ? { 'X-External-Origin-Key': options.externalOriginKey }
                : {}),
              ...(options.octopusImportMode
                ? { 'X-Octopus-Import-Mode': options.octopusImportMode }
                : {}),
              ...(options.replaceOctopusOrigin
                ? { 'X-Octopus-Replace-Origin': 'true' }
                : {}),
              ...(options.replaceAxonHubOrigin
                ? { 'X-AxonHub-Replace-Origin': 'true' }
                : {}),
              ...(options.replaceMagpieOrigin
                ? { 'X-Magpie-Replace-Origin': 'true' }
                : {}),
              ...(options.replacementRevision
                ? {
                    'X-External-Replacement-Revision':
                      options.replacementRevision,
                  }
                : {}),
              ...confirmHeader(options.confirmToken),
            }
          : undefined,
      skipErrorHandler: true,
    }),
  // Import plan preview before commit.
  previewBackupImport: (
    data: unknown,
    options: Pick<
      BackupImportOptions,
      'externalOriginKey' | 'magpiePassphrase'
    > = {}
  ) =>
    request('/api/settings/backup/import/preview', {
      method: 'POST',
      body: backupImportBody(data, options.magpiePassphrase),
      headers: options.externalOriginKey
        ? { 'X-External-Origin-Key': options.externalOriginKey }
        : undefined,
      skipErrorHandler: true,
    }),
  getBackupWebdavConfig: () =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav'),
  saveBackupWebdavConfig: (
    data: Partial<{
      enabled: boolean
      fileUrl: string
      username: string
      password?: string
      clearPassword?: boolean
      exportType: BackupWebdavExportType
      autoSyncEnabled: boolean
      autoSyncCron: string
    }>,
    confirmToken?: string
  ) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav', {
      method: 'PUT',
      body: JSON.stringify(data),
      headers: confirmHeader(confirmToken),
      skipErrorHandler: true,
    }),
  // #1034: sensitive op — master-token re-confirmation via confirmToken.
  exportBackupToWebdav: (
    type?: BackupWebdavExportType,
    confirmToken?: string
  ) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/export', {
      method: 'POST',
      body: JSON.stringify(type ? { type } : {}),
      headers: confirmToken
        ? { 'X-Admin-Confirm-Token': confirmToken }
        : undefined,
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  importBackupFromWebdav: (confirmToken?: string) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/import', {
      method: 'POST',
      body: JSON.stringify({}),
      headers: confirmHeader(confirmToken),
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  clearRuntimeCache: () =>
    request('/api/settings/maintenance/clear-cache', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  clearUsageData: (confirmToken?: string) =>
    request('/api/settings/maintenance/clear-usage', {
      method: 'POST',
      headers: confirmHeader(confirmToken),
      skipErrorHandler: true,
    }),
  factoryReset: (confirmToken?: string) =>
    request('/api/settings/maintenance/factory-reset', {
      method: 'POST',
      headers: confirmHeader(confirmToken),
      skipErrorHandler: true,
    }),
  testNotification: () =>
    request('/api/settings/notify/test', {
      method: 'POST',
      skipErrorHandler: true,
    }),
}

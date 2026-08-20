import {
  fetchAuthenticatedResponse,
  extractResponseErrorMessage,
} from '@/lib/http-client'

import { request, buildQueryString, streamSse } from './transport'
import type {
  BackupWebdavExportType,
  BackupWebdavResponse,
  SystemProxyTestRequest,
  RuntimeSettingsPayload,
  SettingsMigrationPreviewResponse,
  SettingsMigrationApplyResponse,
  DownstreamApiKeyTrendResponse,
} from './types'

export const settingsApi = {
  // Auth management
  getAuthInfo: () => request('/api/settings/auth/info'),
  changeAuthToken: (oldToken: string, newToken: string) =>
    request('/api/settings/auth/change', {
      method: 'POST',
      body: JSON.stringify({ oldToken, newToken }),
      skipErrorHandler: true,
    }),
  getRuntimeSettings: () => request('/api/settings/runtime'),
  getBrandList: () => request('/api/settings/brand-list'),
  updateRuntimeSettings: (data: RuntimeSettingsPayload) =>
    request('/api/settings/runtime', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  getUpdateCenterStatus: () => request('/api/update-center/status'),
  saveUpdateCenterConfig: (data: unknown) =>
    request('/api/update-center/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  checkUpdateCenter: () =>
    request('/api/update-center/check', {
      method: 'POST',
      body: JSON.stringify({}),
    }),
  deployUpdateCenter: (data: {
    source: 'github-release' | 'docker-hub-tag'
    targetTag: string
    targetDigest?: string | null
  }) =>
    request('/api/update-center/deploy', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  rollbackUpdateCenter: (data: { targetRevision: string }) =>
    request('/api/update-center/rollback', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  streamUpdateCenterTaskLogs: (
    taskId: string,
    handlers: {
      onLog?: (entry: unknown) => void
      onDone?: (payload: unknown) => void
      signal?: AbortSignal
    }
  ) =>
    streamSse(
      `/api/update-center/tasks/${encodeURIComponent(taskId)}/stream`,
      handlers
    ),
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
  testExternalDatabaseConnection: (data: {
    dialect: 'sqlite' | 'postgres'
    connectionString: string
    ssl?: boolean
  }) =>
    request('/api/settings/database/test-connection', {
      method: 'POST',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  /**
   * Queue a migration of the live runtime database onto an external target as
   * an admin background task. Resolves with the task ID once the backend has
   * accepted the job (202); progress is observed by polling `eventsApi.getTask`.
   */
  startDatabaseMigration: (data: {
    dialect: 'sqlite' | 'postgres'
    connectionString: string
    overwrite?: boolean
    ssl?: boolean
  }) =>
    request<{ success: boolean; message: string; taskId: string }>(
      '/api/settings/database/migrate',
      {
        method: 'POST',
        body: JSON.stringify(data),
        skipErrorHandler: true,
      }
    ),
  getDownstreamApiKeys: () => request('/api/downstream-keys'),
  createDownstreamApiKey: (data: unknown) =>
    request('/api/downstream-keys', {
      method: 'POST',
      body: JSON.stringify(data),
      // The keys section surfaces its own createFailed toast.
      skipErrorHandler: true,
    }),
  updateDownstreamApiKey: (id: number, data: unknown) =>
    request(`/api/downstream-keys/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
      // The keys section surfaces its own updateFailed toast (form + toggle).
      skipErrorHandler: true,
    }),
  deleteDownstreamApiKey: (id: number) =>
    request(`/api/downstream-keys/${id}`, {
      method: 'DELETE',
      // The keys section surfaces its own deleteFailed toast.
      skipErrorHandler: true,
    }),
  batchDownstreamApiKeys: (data: {
    ids: number[]
    action: 'enable' | 'disable' | 'delete' | 'resetUsage' | 'updateMetadata'
    groupOperation?: 'keep' | 'set' | 'clear'
    groupName?: string
    tagOperation?: 'keep' | 'append'
    tags?: string[]
  }) =>
    request('/api/downstream-keys/batch', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  resetDownstreamApiKeyUsage: (id: number) =>
    request(`/api/downstream-keys/${id}/reset-usage`, { method: 'POST' }),
  getDownstreamKeyExport: (
    id: number,
    profile: 'all' | 'openai' | 'cherry' | 'generic' = 'all'
  ) => request(`/api/downstream-keys/${id}/export?profile=${profile}`),
  getDownstreamApiKeysSummary: (params?: {
    range?: '24h' | '7d' | 'all'
    status?: 'all' | 'enabled' | 'disabled'
    search?: string
  }) => request(`/api/downstream-keys/summary${buildQueryString(params)}`),
  getDownstreamApiKeyOverview: (id: number) =>
    request(`/api/downstream-keys/${id}/overview`),
  getDownstreamApiKeyTrend: (
    id: number,
    params?: { range?: '24h' | '7d' | 'all'; timeZone?: string }
  ) =>
    request<DownstreamApiKeyTrendResponse>(
      `/api/downstream-keys/${id}/trend${buildQueryString(params)}`
    ),
  exportBackup: (type: 'all' | 'accounts' | 'preferences' = 'all') =>
    request(`/api/settings/backup/export?type=${encodeURIComponent(type)}`),
  /** Raw text export for file download; throws on non-OK responses. */
  exportBackupRaw: async (type: 'all' | 'accounts' | 'preferences' = 'all') => {
    const response = await fetchAuthenticatedResponse(
      `/api/settings/backup/export?type=${encodeURIComponent(type)}`
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
  importBackup: (data: unknown) =>
    request('/api/settings/backup/import', {
      method: 'POST',
      body: JSON.stringify({ data }),
      skipErrorHandler: true,
    }),
  // F1: import plan preview before commit.
  previewBackupImport: (data: unknown) =>
    request('/api/settings/backup/import/preview', {
      method: 'POST',
      body: JSON.stringify({ data }),
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
    }>
  ) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav', {
      method: 'PUT',
      body: JSON.stringify(data),
      skipErrorHandler: true,
    }),
  exportBackupToWebdav: (type?: BackupWebdavExportType) =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/export', {
      method: 'POST',
      body: JSON.stringify(type ? { type } : {}),
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  importBackupFromWebdav: () =>
    request<BackupWebdavResponse>('/api/settings/backup/webdav/import', {
      method: 'POST',
      body: JSON.stringify({}),
      timeoutMs: 60_000,
      skipErrorHandler: true,
    }),
  clearRuntimeCache: () =>
    request('/api/settings/maintenance/clear-cache', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  clearUsageData: () =>
    request('/api/settings/maintenance/clear-usage', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  factoryReset: () =>
    request('/api/settings/maintenance/factory-reset', {
      method: 'POST',
      skipErrorHandler: true,
    }),
  testNotification: () =>
    request('/api/settings/notify/test', {
      method: 'POST',
      skipErrorHandler: true,
    }),
}

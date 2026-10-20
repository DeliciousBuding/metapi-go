// Behavior test for import/export sensitive writes (#1034).
//
// Backup import, WebDAV config save and WebDAV import all write
// credential-bearing state. When the backend answers `403 reauthRequired`,
// the section must open the master-token dialog and replay the pending
// request exactly once with the token; cancellation must leave the backend
// untouched.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'

import { ImportExportSection } from '../import-export-section'

const {
  mockGetBackupWebdavConfig,
  mockImportBackup,
  mockImportBackupFromWebdav,
  mockPreviewBackupImport,
  mockSaveBackupWebdavConfig,
} = vi.hoisted(() => ({
  mockGetBackupWebdavConfig: vi.fn(),
  mockImportBackup: vi.fn(),
  mockImportBackupFromWebdav: vi.fn(),
  mockPreviewBackupImport: vi.fn(),
  mockSaveBackupWebdavConfig: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getBackupWebdavConfig: mockGetBackupWebdavConfig,
    importBackup: mockImportBackup,
    importBackupFromWebdav: mockImportBackupFromWebdav,
    previewBackupImport: mockPreviewBackupImport,
    saveBackupWebdavConfig: mockSaveBackupWebdavConfig,
    exportBackupRaw: vi.fn(),
    exportBackupToWebdav: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

// FormNavigationGuard is the only router consumer in this tree; stub the
// blocker so the section renders without a router context.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle' }),
}))

const webdavConfig = {
  enabled: true,
  fileUrl: 'https://dav.example.com/backups/metapi.json',
  username: 'dav-user',
  hasPassword: true,
  passwordMasked: '••••••••',
  exportType: 'all',
  autoSyncEnabled: false,
  autoSyncCron: '0 */6 * * *',
}

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

beforeEach(() => {
  mockGetBackupWebdavConfig.mockReset()
  mockImportBackup.mockReset()
  mockImportBackupFromWebdav.mockReset()
  mockPreviewBackupImport.mockReset()
  mockSaveBackupWebdavConfig.mockReset()
  mockGetBackupWebdavConfig.mockResolvedValue({
    success: true,
    ...webdavConfig,
    config: webdavConfig,
    state: {},
  })
  mockPreviewBackupImport.mockResolvedValue({
    success: true,
    plan: { accounts: { rows: 1, toInsert: 1, duplicates: 0, skippedRows: 0 } },
  })
})

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ReauthProvider>
        <ImportExportSection />
      </ReauthProvider>
    </QueryClientProvider>
  )
}

function clickLastButtonNamed(name: string) {
  const buttons = screen.getAllByRole('button', { name })
  const button = buttons.at(-1)
  if (!button) throw new Error(`Button "${name}" not found`)
  fireEvent.click(button)
}

async function confirmReauth(token: string) {
  const input = await screen.findByLabelText(text('reauth.tokenLabel'))
  fireEvent.change(input, { target: { value: token } })
  fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))
}

describe('ImportExportSection — master-token replay', () => {
  it('replays a backup import with the token', async () => {
    mockImportBackup
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true })
    renderSection()

    fireEvent.click(
      screen.getByRole('button', { name: 'Advanced · paste JSON' })
    )
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: '{"version":1}' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )
    await screen.findByRole('region', { name: 'Import plan' })
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')

    await waitFor(() => expect(mockImportBackup).toHaveBeenCalledTimes(1))
    // The unconfirmed attempt carries no confirmation token.
    expect(mockImportBackup.mock.calls[0]).toHaveLength(2)
    expect(mockImportBackup.mock.calls[0]?.[1]).not.toHaveProperty(
      'confirmToken'
    )

    await confirmReauth('master-token')

    await waitFor(() => expect(mockImportBackup).toHaveBeenCalledTimes(2))
    expect(mockImportBackup.mock.calls[1]).toHaveLength(2)
    expect(mockImportBackup.mock.calls[1]?.[1]).toMatchObject({
      confirmToken: 'master-token',
    })
  })

  it('replays a WebDAV config save with the token', async () => {
    mockSaveBackupWebdavConfig
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true, ...webdavConfig })
    renderSection()

    const fileUrl = await screen.findByLabelText('File URL')
    fireEvent.change(fileUrl, {
      target: { value: 'https://dav.example.com/new-backup.json' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save WebDAV config' }))

    await waitFor(() =>
      expect(mockSaveBackupWebdavConfig).toHaveBeenCalledTimes(1)
    )
    expect(mockSaveBackupWebdavConfig.mock.calls[0]).toHaveLength(1)

    await confirmReauth('master-token')

    await waitFor(() =>
      expect(mockSaveBackupWebdavConfig).toHaveBeenCalledTimes(2)
    )
    expect(mockSaveBackupWebdavConfig).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        fileUrl: 'https://dav.example.com/new-backup.json',
      }),
      'master-token'
    )
  })

  it('replays a WebDAV import with the token', async () => {
    mockImportBackupFromWebdav
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true })
    renderSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Import from WebDAV' })
    )
    await screen.findByText('Import backup from WebDAV?')
    clickLastButtonNamed('Import from WebDAV')

    await waitFor(() =>
      expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(1)
    )
    expect(mockImportBackupFromWebdav).toHaveBeenCalledWith()

    await confirmReauth('master-token')

    await waitFor(() =>
      expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(2)
    )
    expect(mockImportBackupFromWebdav).toHaveBeenNthCalledWith(
      2,
      'master-token'
    )
  })

  it('leaves the backend untouched when the reauth prompt is cancelled', async () => {
    mockImportBackupFromWebdav.mockRejectedValueOnce(reauthError())
    renderSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Import from WebDAV' })
    )
    await screen.findByText('Import backup from WebDAV?')
    clickLastButtonNamed('Import from WebDAV')

    await waitFor(() =>
      expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(1)
    )
    await screen.findByLabelText(text('reauth.tokenLabel'))
    fireEvent.click(screen.getByRole('button', { name: text('reauth.cancel') }))

    await waitFor(() =>
      expect(
        screen.queryByLabelText(text('reauth.tokenLabel'))
      ).not.toBeInTheDocument()
    )
    expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(1)
  })
})

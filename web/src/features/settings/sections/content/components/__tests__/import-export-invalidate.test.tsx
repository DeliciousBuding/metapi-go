// Regression test: backup imports must invalidate every cached domain
// (sites / accounts / attention / settings / webdav) so list pages and
// dashboard widgets refresh after the import lands (audit #1029 batch B).
// Previously the success toast fired but stale queries kept showing
// pre-import data until a manual refresh.
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

import '@/i18n/config'

import { ImportExportSection } from '../import-export-section'

const {
  mockGetBackupWebdavConfig,
  mockImportBackup,
  mockImportBackupFromWebdav,
  mockPreviewBackupImport,
} = vi.hoisted(() => ({
  mockGetBackupWebdavConfig: vi.fn(),
  mockImportBackup: vi.fn(),
  mockImportBackupFromWebdav: vi.fn(),
  mockPreviewBackupImport: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getBackupWebdavConfig: mockGetBackupWebdavConfig,
    importBackup: mockImportBackup,
    importBackupFromWebdav: mockImportBackupFromWebdav,
    previewBackupImport: mockPreviewBackupImport,
    exportBackupRaw: vi.fn(),
    exportBackupToWebdav: vi.fn(),
    saveBackupWebdavConfig: vi.fn(),
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
  // base-ui AlertDialog/Select query matchMedia under jsdom.
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
  mockGetBackupWebdavConfig.mockResolvedValue({
    success: true,
    ...webdavConfig,
    config: webdavConfig,
    state: {},
  })
  mockImportBackup.mockResolvedValue({ success: true })
  mockImportBackupFromWebdav.mockResolvedValue({ success: true })
  mockPreviewBackupImport.mockResolvedValue({
    success: true,
    plan: { accounts: { rows: 1, toInsert: 1, duplicates: 0, skippedRows: 0 } },
  })
})

afterEach(() => cleanup())

function renderImportExportSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  render(
    <QueryClientProvider client={queryClient}>
      <ImportExportSection />
    </QueryClientProvider>
  )
  return { invalidateSpy }
}

// The confirm dialog action shares the trigger's label; the dialog's action
// is the last matching button in the tree.
function clickLastButtonNamed(name: string) {
  const buttons = screen.getAllByRole('button', { name })
  const button = buttons.at(-1)
  if (!button) {
    throw new Error(`Button "${name}" not found`)
  }
  fireEvent.click(button)
}

const EXPECTED_KEYS = [
  ['sites', 'list'],
  ['accounts'],
  ['dashboard-attention'],
  ['settings-auth-info'],
  ['backup-webdav'],
]

function expectInvalidated(invalidateSpy: ReturnType<typeof vi.spyOn>) {
  for (const key of EXPECTED_KEYS) {
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: key })
  }
}

describe('ImportExportSection — cache invalidation after import', () => {
  it('invalidates domain queries after a pasted backup import', async () => {
    const { invalidateSpy } = renderImportExportSection()

    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: '{"version":1}' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')

    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => expectInvalidated(invalidateSpy))
  })

  it('invalidates domain queries after a WebDAV backup import', async () => {
    const { invalidateSpy } = renderImportExportSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Import from WebDAV' })
    )
    await screen.findByText('Import backup from WebDAV?')
    clickLastButtonNamed('Import from WebDAV')

    await waitFor(() => {
      expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => expectInvalidated(invalidateSpy))
  })
})

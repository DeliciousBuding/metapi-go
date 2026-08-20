// Regression test for the WebDAV backup import. The button used to fire the
// one-click overwrite mutation directly; it must now open a destructive
// confirmation first and only import after the operator confirms (issue #889).
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
  mockImportBackupFromWebdav,
  mockToastSuccess,
  mockToastError,
} = vi.hoisted(() => ({
  mockGetBackupWebdavConfig: vi.fn(),
  mockImportBackupFromWebdav: vi.fn(),
  mockToastSuccess: vi.fn(),
  mockToastError: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getBackupWebdavConfig: mockGetBackupWebdavConfig,
    importBackupFromWebdav: mockImportBackupFromWebdav,
    exportBackupRaw: vi.fn(),
    exportBackupToWebdav: vi.fn(),
    previewBackupImport: vi.fn(),
    importBackup: vi.fn(),
    saveBackupWebdavConfig: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
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
  mockImportBackupFromWebdav.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockGetBackupWebdavConfig.mockResolvedValue({
    success: true,
    ...webdavConfig,
    config: webdavConfig,
    state: {},
  })
  mockImportBackupFromWebdav.mockResolvedValue({ success: true })
})

afterEach(() => cleanup())

function renderImportExportSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ImportExportSection />
    </QueryClientProvider>
  )
}

// The confirm dialog action shares the trigger's label; the dialog's action
// is the last matching button in the tree.
function clickConfirmAction() {
  const confirmButtons = screen.getAllByRole('button', {
    name: 'Import from WebDAV',
  })
  const confirmButton = confirmButtons.at(-1)
  if (!confirmButton) {
    throw new Error('Confirm action button not found')
  }
  fireEvent.click(confirmButton)
}

describe('ImportExportSection — WebDAV import guard', () => {
  it('requires a confirmation before importing the WebDAV backup', async () => {
    renderImportExportSection()

    const importButton = await screen.findByRole('button', {
      name: 'Import from WebDAV',
    })
    fireEvent.click(importButton)

    expect(
      await screen.findByText('Import backup from WebDAV?')
    ).toBeInTheDocument()
    expect(mockImportBackupFromWebdav).not.toHaveBeenCalled()

    clickConfirmAction()

    await waitFor(() => {
      expect(mockImportBackupFromWebdav).toHaveBeenCalledTimes(1)
    })
  })

  it('does not import when the confirmation is cancelled', async () => {
    renderImportExportSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Import from WebDAV' })
    )
    await screen.findByText('Import backup from WebDAV?')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(mockImportBackupFromWebdav).not.toHaveBeenCalled()
  })
})

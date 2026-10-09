// Regression test: backup imports must invalidate the whole query cache so
// every list page / dashboard widget / settings section refreshes after the
// import lands. An import merges rows across the entire schema (~30 tables —
// sites, accounts, tokens, routes, channels, check-in, OAuth, settings, …),
// so the handler invalidates everything instead of a hand-maintained key list
// that kept missing domains (#1029).
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
  fireEvent.click(screen.getByRole('button', { name: 'Advanced · paste JSON' }))
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

function expectInvalidated(invalidateSpy: ReturnType<typeof vi.spyOn>) {
  // Blanket invalidation — a call with no filter argument invalidates the
  // entire query cache, which is the required behaviour after an import.
  expect(invalidateSpy).toHaveBeenCalledWith()
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

    await screen.findByRole('region', { name: 'Import plan' })
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(mockImportBackup).not.toHaveBeenCalled()
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')

    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => expectInvalidated(invalidateSpy))
  })

  it('previews and commits Octopus v5 with one stable external origin key', async () => {
    const { invalidateSpy } = renderImportExportSection()
    const payload = {
      version: 5,
      exported_at: '2026-10-06T00:00:00Z',
      channels: [],
    }
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'octopus-lab',
        sections: { channels: 2, statsRecords: 4 },
        notImported: {},
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify(payload) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'octopus-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    expect(
      (await screen.findByText('Channels')).parentElement
    ).toHaveTextContent('2')
    expect(screen.getByText('History records').parentElement).toHaveTextContent(
      '4'
    )
    expect(mockPreviewBackupImport).toHaveBeenCalledWith(payload, 'octopus-lab')
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')

    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledWith(
        payload,
        'octopus-lab',
        undefined,
        false,
        false
      )
    })
    await waitFor(() => expectInvalidated(invalidateSpy))
  })

  it('discards stale async previews and commits only the reviewed payload snapshot', async () => {
    renderImportExportSection()
    let resolvePreview!: (value: unknown) => void
    mockPreviewBackupImport.mockReturnValueOnce(
      new Promise((resolve) => {
        resolvePreview = resolve
      })
    )
    const input = screen.getByPlaceholderText('{ "version": "..." }')
    const payloadA = { version: 5, exported_at: 'A', channels: [] }
    const payloadB = { version: 5, exported_at: 'B', channels: [] }
    fireEvent.change(input, { target: { value: JSON.stringify(payloadA) } })
    const origin = await screen.findByLabelText('External origin key')
    fireEvent.change(origin, { target: { value: 'origin-A' } })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )
    await waitFor(() =>
      expect(mockPreviewBackupImport).toHaveBeenCalledTimes(1)
    )

    fireEvent.change(input, { target: { value: JSON.stringify(payloadB) } })
    fireEvent.change(origin, { target: { value: 'origin-B' } })
    resolvePreview({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'origin-A',
        sections: { channels: 1 },
      },
    })
    await waitFor(() => {
      expect(screen.queryByText('Confirm import?')).not.toBeInTheDocument()
      expect(
        screen.queryByText('Source: octopus-v5 · origin key: origin-A')
      ).not.toBeInTheDocument()
    })

    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'origin-B',
        sections: { channels: 1 },
      },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )
    await screen.findByText('Source: octopus-v5 · origin key: origin-B')
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledWith(
        payloadB,
        'origin-B',
        undefined,
        false,
        false
      )
    })
  })

  it('requires explicit channels-only acknowledgement for unsupported Octopus sections', async () => {
    renderImportExportSection()
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'octopus-lab',
        sections: { channels: 1 },
        notImported: { settings: 2 },
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify({ version: 5, exported_at: 'now' }) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'octopus-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    expect(
      await screen.findByText(
        'Not migrated (will remain only in the source file):'
      )
    ).toBeInTheDocument()
    const acknowledgement = screen.getByRole('checkbox', {
      name: /I understand: import channels, credentials, model grants/i,
    })
    fireEvent.click(acknowledgement)
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledWith(
        { version: 5, exported_at: 'now' },
        'octopus-lab',
        'channels-only',
        false,
        false
      )
    })
  })

  it('requires explicit acknowledgement for default group relay policy adaptation', async () => {
    renderImportExportSection()
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'octopus-lab',
        sections: { channels: 1 },
        adaptations: ['groupRelayConfigDefaults'],
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify({ version: 5, exported_at: 'now' }) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'octopus-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    expect(
      await screen.findByText(/Metapi's routing rules will apply instead/)
    ).toBeInTheDocument()
    expect(screen.queryByText('Confirm import?')).not.toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: /Default group retry, timeout, cooldown, and affinity policies/i,
      })
    )
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledWith(
        { version: 5, exported_at: 'now' },
        'octopus-lab',
        'channels-only',
        false,
        false
      )
    })
  })

  it('confirms origin removals only after reviewing the replacement notice', async () => {
    renderImportExportSection()
    const payload = { version: 5, exported_at: 'replacement' }
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'octopus-v5',
        originKey: 'octopus-lab',
        sections: { channels: 1 },
        removals: { channelGrants: 1 },
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify(payload) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'octopus-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )
    await screen.findByText('Remove entries deleted from this source')
    expect(mockImportBackup).not.toHaveBeenCalled()
    clickLastButtonNamed('Import')
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent(
      'Native data and other origins are unaffected'
    )
    expect(mockImportBackup).not.toHaveBeenCalled()
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledExactlyOnceWith(
        payload,
        'octopus-lab',
        undefined,
        true,
        false
      )
    })
  })

  it('rejects an Octopus preview response without a plan', async () => {
    renderImportExportSection()
    mockPreviewBackupImport.mockResolvedValueOnce({ success: true })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify({ version: 5, exported_at: 'now' }) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'octopus-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    await waitFor(() => expect(mockImportBackup).not.toHaveBeenCalled())
    expect(screen.queryByText('Confirm import?')).not.toBeInTheDocument()
  })

  it('previews and commits an AxonHub v1.4 channel graph', async () => {
    const { invalidateSpy } = renderImportExportSection()
    const payload = {
      version: '1.4',
      timestamp: '2026-01-01T00:00:00Z',
      channels: [],
      models: [],
    }
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'axonhub-v1.4',
        originKey: 'axonhub-lab',
        version: '1.4',
        sections: { channels: 3, models: 7 },
        routable: {
          channels: 3,
          credentials: 4,
          models: 5,
          grants: 6,
          routes: 5,
        },
        notImported: { projects: 1, apiKeys: 1 },
        skippedChannels: [
          {
            sourceId: 3,
            type: 'gemini',
            reasons: ['provider_translation_unsupported'],
          },
        ],
        residuals: ['channel-4:manual_models_not_imported_until_synced'],
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify(payload) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'axonhub-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    expect(
      await screen.findByText('Channel graph that will be written:')
    ).toBeInTheDocument()
    expect(
      screen.getByText('Channels that cannot be imported:')
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        'requires a protocol translation this gateway does not implement'
      )
    ).toBeInTheDocument()
    expect(
      screen.getByText('Source settings replaced by a Metapi equivalent:')
    ).toBeInTheDocument()
    // An AxonHub import never asks for the Octopus channels-only acknowledgement.
    expect(
      screen.queryByRole('checkbox', {
        name: /I understand: import channels, credentials, model grants/i,
      })
    ).not.toBeInTheDocument()
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledExactlyOnceWith(
        payload,
        'axonhub-lab',
        undefined,
        false,
        false
      )
    })
    await waitFor(() => expectInvalidated(invalidateSpy))
  })

  it('confirms AxonHub origin removals with the AxonHub replacement header only', async () => {
    renderImportExportSection()
    const payload = {
      version: '1.4',
      timestamp: '2026-01-01T00:00:00Z',
      channels: [],
      models: [],
    }
    mockPreviewBackupImport.mockResolvedValueOnce({
      success: true,
      plan: {
        source: 'axonhub-v1.4',
        originKey: 'axonhub-lab',
        version: '1.4',
        sections: { channels: 2 },
        routable: { channels: 2, routes: 3 },
        removals: { token_routes: 2 },
      },
    })
    fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
      target: { value: JSON.stringify(payload) },
    })
    fireEvent.change(screen.getByLabelText('External origin key'), {
      target: { value: 'axonhub-lab' },
    })
    fireEvent.click(
      await screen.findByRole('button', { name: 'Preview import' })
    )

    await screen.findByText('Remove entries deleted from this source')
    clickLastButtonNamed('Import')
    await screen.findByText('Confirm import?')
    clickLastButtonNamed('Import')
    await waitFor(() => {
      expect(mockImportBackup).toHaveBeenCalledExactlyOnceWith(
        payload,
        'axonhub-lab',
        undefined,
        false,
        true
      )
    })
  })

  it('loads a selected JSON backup file into the import preview editor', async () => {
    renderImportExportSection()
    const file = new File(
      [JSON.stringify({ tables: { settings: [] } })],
      'backup.json',
      { type: 'application/json' }
    )
    fireEvent.change(screen.getByLabelText('Choose a JSON backup file'), {
      target: { files: [file] },
    })
    await waitFor(() => {
      expect(
        (
          screen.getByPlaceholderText(
            '{ "version": "..." }'
          ) as HTMLTextAreaElement
        ).value
      ).toContain('"settings":[]')
    })
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

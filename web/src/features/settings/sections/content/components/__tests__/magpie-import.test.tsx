import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ImportExportSection } from '../import-export-section'

const { previewBackupImport, importBackup } = vi.hoisted(() => ({
  previewBackupImport: vi.fn(),
  importBackup: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    previewBackupImport,
    importBackup,
    getBackupWebdavConfig: vi.fn().mockResolvedValue({
      config: {
        enabled: false,
        fileUrl: '',
        username: '',
        exportType: 'all',
        autoSyncEnabled: false,
        autoSyncCron: '0 */6 * * *',
      },
      state: {},
    }),
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle' }),
}))

const envelope = {
  format: 'magpie-backup',
  version: 1,
  kdf: 'pbkdf2-sha256',
  iterations: 600_000,
  salt: 'c2FsdA==',
  nonce: 'bm9uY2U=',
  data: 'Y2lwaGVydGV4dA==',
}

const preview = {
  success: true,
  plan: {
    source: 'magpie-backup',
    originKey: 'magpie-backup',
    version: 'bundle-2',
    sections: { providers: 1, gatewayKeys: 1 },
    routable: { channels: 1, routes: 2, groups: 2, gatewayKeys: 1 },
    residuals: ['group_match_expanded_to_snapshot'],
    removals: { downstreamKeys: 1 },
    removalImpact: {
      kind: 'source',
      id: 0,
      counts: { downstreamKeys: 1 },
      affectedRouteIds: [],
      revision: 'magpie-review',
      requiresCascade: false,
    },
  },
}

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportExportSection />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Advanced · paste JSON' }))
}

function pasteEnvelope() {
  fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
    target: { value: JSON.stringify(envelope) },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  previewBackupImport.mockReset().mockResolvedValue(preview)
  importBackup.mockReset().mockResolvedValue({ success: true })
  localStorage.clear()
  window.history.replaceState(null, '', '/settings')
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

afterEach(cleanup)

describe('Magpie backup import', () => {
  it('loads a .magpie-backup file saved with a UTF-8 BOM', async () => {
    renderSection()
    const file = new File(
      [`\uFEFF${JSON.stringify(envelope)}`],
      'fixture.magpie-backup',
      { type: 'application/json' }
    )
    fireEvent.change(screen.getByLabelText('Choose a JSON backup file'), {
      target: { files: [file] },
    })
    expect(
      await screen.findByLabelText('Magpie backup passphrase')
    ).toBeVisible()
    expect(screen.getByLabelText('External origin key')).toHaveValue(
      'magpie-backup'
    )
  })

  it('detects the base64 envelope and requires a passphrase before preview', async () => {
    renderSection()
    pasteEnvelope()

    const passphrase = await screen.findByLabelText('Magpie backup passphrase')
    expect(passphrase).toHaveAttribute('type', 'password')
    expect(screen.getByLabelText('External origin key')).toHaveValue(
      'magpie-backup'
    )

    expect(
      screen.getByRole('button', { name: 'Preview import' })
    ).toBeDisabled()
    expect(previewBackupImport).not.toHaveBeenCalled()
  })

  it('sends the passphrase only in the request and confirms replacement explicitly', async () => {
    renderSection()
    pasteEnvelope()
    fireEvent.change(await screen.findByLabelText('Magpie backup passphrase'), {
      target: { value: 'fixture-passphrase' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Preview import' }))

    const panel = await screen.findByRole('region', { name: 'Import plan' })
    expect(panel).toHaveTextContent(
      'Magpie capabilities and policy differences'
    )
    await waitFor(() =>
      expect(previewBackupImport).toHaveBeenCalledExactlyOnceWith(envelope, {
        externalOriginKey: 'magpie-backup',
        magpiePassphrase: 'fixture-passphrase',
      })
    )
    expect(localStorage.length).toBe(0)
    expect(window.location.href).not.toContain('fixture-passphrase')

    fireEvent.click(screen.getByRole('button', { name: 'Import' }))
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog).toHaveTextContent('Writes the migratable Magpie providers')
    fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
    expect(importBackup).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Import' }))
    const confirmed = await screen.findByRole('alertdialog')
    fireEvent.click(within(confirmed).getByRole('button', { name: 'Import' }))
    await waitFor(() =>
      expect(importBackup).toHaveBeenCalledExactlyOnceWith(envelope, {
        externalOriginKey: 'magpie-backup',
        octopusImportMode: undefined,
        replaceOctopusOrigin: false,
        replaceAxonHubOrigin: false,
        replaceMagpieOrigin: true,
        replacementRevision: 'magpie-review',
        magpiePassphrase: 'fixture-passphrase',
      })
    )
    expect(localStorage.length).toBe(0)
  })
})

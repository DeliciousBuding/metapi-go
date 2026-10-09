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
import { toast } from '@/lib/toast'

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

const octopus = { version: 5, exported_at: 'first', channels: [] }
const axonhub = { version: '1.4', timestamp: 'first', channels: [], models: [] }

function preview(
  revision = 'review-1',
  source = 'octopus-v5',
  originKey = 'source'
) {
  return {
    success: true,
    plan: {
      source,
      originKey,
      version: '1.4',
      sections: { channels: 1 },
      routable: {},
      // The legacy source-only count is smaller than the actual closure.
      removals: { channelGrants: 1 },
      removalImpact: {
        kind: 'source',
        id: 0,
        counts: {
          channels: 1,
          models: 2,
          credentials: 3,
          grants: 7,
          groups: 4,
          members: 5,
          routes: 6,
          routeChannels: 8,
          routeGroupSources: 9,
          downstreamKeys: 10,
          sourceMappings: 12345,
        },
        affectedRouteIds: [6],
        revision,
        requiresCascade: true,
      },
    },
  }
}

function renderSection(payload: unknown = octopus) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      // The destructive import must explicitly override application retries.
      mutations: { retry: 2, retryDelay: 0 },
    },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ImportExportSection />
    </QueryClientProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Advanced · paste JSON' }))
  fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
    target: { value: JSON.stringify(payload) },
  })
  fireEvent.change(screen.getByLabelText('External origin key'), {
    target: { value: 'source' },
  })
}

async function reviewPreview() {
  fireEvent.click(screen.getByRole('button', { name: 'Preview import' }))
  return screen.findByRole('region', { name: 'Import plan' })
}

async function openConfirmation() {
  fireEvent.click(screen.getByRole('button', { name: 'Import' }))
  return screen.findByRole('alertdialog')
}

async function confirmImport() {
  const dialog = await openConfirmation()
  fireEvent.click(within(dialog).getByRole('button', { name: 'Import' }))
}

beforeEach(() => {
  vi.clearAllMocks()
  previewBackupImport.mockReset().mockResolvedValue(preview())
  importBackup.mockReset().mockResolvedValue({ success: true })
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

describe('external source replacement review', () => {
  it.each([true, false])(
    'shows AxonHub key deletion separately from affected route access (keys only: %s)',
    async (keysOnly) => {
      const result = preview('keys-revision', 'axonhub-v1.4')
      previewBackupImport.mockResolvedValue({
        ...result,
        plan: {
          ...result.plan,
          removals: { downstream_api_keys: 3 },
          removalImpact: {
            ...result.plan.removalImpact,
            counts: keysOnly ? {} : result.plan.removalImpact.counts,
          },
        },
      })
      renderSection(axonhub)
      const panel = await reviewPreview()
      expect(panel).toHaveTextContent('Downstream keys to delete · 3')
      if (!keysOnly) {
        expect(panel).toHaveTextContent('Keys with affected route access · 10')
      }
      const dialog = await openConfirmation()
      expect(dialog).toHaveTextContent('Downstream keys to delete: 3 record(s)')
      fireEvent.click(within(dialog).getByRole('button', { name: 'Import' }))
      await waitFor(() =>
        expect(importBackup).toHaveBeenCalledExactlyOnceWith(
          axonhub,
          'source',
          undefined,
          false,
          true,
          'keys-revision'
        )
      )
    }
  )

  it.each([
    { source: 'octopus-v5', payload: octopus },
    { source: 'axonhub-v1.4', payload: axonhub },
  ])(
    'shows the actual closure and binds $source confirmation to its revision',
    async ({ source, payload }) => {
      previewBackupImport.mockResolvedValue(preview('reviewed', source))
      renderSection(payload)
      const panel = await reviewPreview()
      expect(panel).toHaveTextContent('Deletion and route access impact')
      expect(panel).toHaveTextContent('Model grants · 7')
      expect(panel).toHaveTextContent('Route channel links · 8')
      expect(panel).toHaveTextContent('Route group links · 9')
      expect(panel).toHaveTextContent('Keys with affected route access · 10')
      expect(panel).toHaveTextContent(
        'including any created locally or by another source'
      )
      expect(panel).not.toHaveTextContent('sourceMappings')
      expect(panel).not.toHaveTextContent('12345')
      expect(panel).not.toHaveTextContent('unaffected')
      const dialog = await openConfirmation()
      expect(dialog).toHaveTextContent('Model grants: 7 record(s)')
      expect(dialog).toHaveTextContent(
        'including any created locally or by another source'
      )
      expect(dialog).not.toHaveTextContent('12345')
      fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }))
      expect(importBackup).not.toHaveBeenCalled()
      await confirmImport()
      await waitFor(() =>
        expect(importBackup).toHaveBeenCalledExactlyOnceWith(
          payload,
          'source',
          undefined,
          source === 'octopus-v5',
          source === 'axonhub-v1.4',
          'reviewed'
        )
      )
    }
  )

  it.each(['text', 'file', 'origin'] as const)(
    'clears open confirmation when the %s changes',
    async (change) => {
      renderSection()
      await reviewPreview()
      await openConfirmation()
      const replacement = { ...octopus, exported_at: 'second' }
      let nextPayload = replacement
      let nextOrigin = 'source'
      if (change === 'text') {
        fireEvent.change(screen.getByPlaceholderText('{ "version": "..." }'), {
          target: { value: JSON.stringify(replacement) },
        })
      } else if (change === 'file') {
        const file = new File([JSON.stringify(replacement)], 'second.json', {
          type: 'application/json',
        })
        fireEvent.change(screen.getByLabelText('Choose a JSON backup file'), {
          target: { files: [file] },
        })
        await waitFor(() =>
          expect(
            screen.getByPlaceholderText('{ "version": "..." }')
          ).toHaveValue(JSON.stringify(replacement))
        )
      } else {
        nextPayload = octopus
        nextOrigin = 'other'
        fireEvent.change(screen.getByLabelText('External origin key'), {
          target: { value: nextOrigin },
        })
      }
      expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
      expect(
        screen.queryByRole('region', { name: 'Import plan' })
      ).not.toBeInTheDocument()
      expect(importBackup).not.toHaveBeenCalled()
      previewBackupImport.mockResolvedValue(
        preview('review-2', 'octopus-v5', nextOrigin)
      )
      await reviewPreview()
      await confirmImport()
      await waitFor(() =>
        expect(importBackup).toHaveBeenCalledExactlyOnceWith(
          nextPayload,
          nextOrigin,
          undefined,
          true,
          false,
          'review-2'
        )
      )
    }
  )

  it('refreshes a 409 preview without retrying import or carrying confirmation forward', async () => {
    const first = {
      ...preview(),
      plan: { ...preview().plan, notImported: { apiKeys: 1 } },
    }
    previewBackupImport.mockResolvedValueOnce(first)
    let resolveRefresh!: (value: unknown) => void
    previewBackupImport.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRefresh = resolve
        })
    )
    importBackup.mockRejectedValueOnce({
      isAxiosError: true,
      response: { status: 409 },
    })
    renderSection()
    await reviewPreview()
    fireEvent.click(screen.getByRole('checkbox', { name: /I understand/ }))
    await confirmImport()
    await waitFor(() => expect(previewBackupImport).toHaveBeenCalledTimes(2))
    expect(importBackup).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Import plan' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Import' })
    ).not.toBeInTheDocument()
    expect(toast.error).toHaveBeenCalledWith(
      'Data changed. Review a fresh preview and confirm again before importing.'
    )
    resolveRefresh({
      ...first,
      plan: {
        ...first.plan,
        removalImpact: { ...first.plan.removalImpact, revision: 'review-2' },
      },
    })
    await screen.findByRole('region', { name: 'Import plan' })
    expect(
      screen.getByRole('checkbox', { name: /I understand/ })
    ).not.toBeChecked()
    expect(screen.getByRole('button', { name: 'Import' })).toBeDisabled()
    expect(importBackup).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('checkbox', { name: /I understand/ }))
    await confirmImport()
    await waitFor(() =>
      expect(importBackup).toHaveBeenLastCalledWith(
        octopus,
        'source',
        'channels-only',
        true,
        false,
        'review-2'
      )
    )
    expect(importBackup).toHaveBeenCalledTimes(2)
  })

  it('does not retain the old preview if a fresh preview fails', async () => {
    renderSection()
    await reviewPreview()
    previewBackupImport.mockRejectedValue(new Error('preview unavailable'))
    fireEvent.click(screen.getByRole('button', { name: 'Preview import' }))
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Import failed.')
    )
    expect(
      screen.queryByRole('region', { name: 'Import plan' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Import' })
    ).not.toBeInTheDocument()
    expect(importBackup).not.toHaveBeenCalled()
  })

  it('requires a nonempty revision when the backend provides deletion impact', async () => {
    previewBackupImport.mockResolvedValue(preview(''))
    renderSection()
    fireEvent.click(screen.getByRole('button', { name: 'Preview import' }))
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith('Import failed.')
    )
    expect(
      screen.queryByRole('region', { name: 'Import plan' })
    ).not.toBeInTheDocument()
    expect(importBackup).not.toHaveBeenCalled()
  })
})

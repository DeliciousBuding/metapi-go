// Behavior tests for the import wizard. Mocks the detect/import mutations
// and the sites list so each test exercises one user-visible wizard behavior
// without touching the network. Asserts only what the user sees (toasts,
// step transitions, rendered result text) — never internal state.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { ImportWizardDialog } from '../components/import-wizard-dialog'

const { mockDetectMutate, mockImportMutate, mockToastError } = vi.hoisted(() => ({
  mockDetectMutate: vi.fn(),
  mockImportMutate: vi.fn(),
  mockToastError: vi.fn(),
}))

vi.mock('../api', () => ({
  useDetectSite: () => ({ mutateAsync: mockDetectMutate, isPending: false }),
  useImportSites: () => ({ mutateAsync: mockImportMutate, isPending: false }),
}))

vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: [], isPending: false }),
}))

vi.mock('sonner', () => ({
  toast: {
    error: mockToastError,
    success: vi.fn(),
  },
}))

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
  mockDetectMutate.mockReset()
  mockImportMutate.mockReset()
  mockToastError.mockReset()
  // Default: detection returns undetectable (empty platform).
  mockDetectMutate.mockResolvedValue({})
})

afterEach(() => cleanup())

describe('ImportWizardDialog', () => {
  it('shows a toast and stays on source when no URLs are pasted', async () => {
    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Next' })).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(mockToastError).toHaveBeenCalledTimes(1)
    // Still on source: the textarea label stays visible.
    expect(screen.getByLabelText('Site URLs')).toBeInTheDocument()
  })

  it('advances from source to identify when URLs are provided', async () => {
    mockDetectMutate.mockResolvedValue({
      platform: 'new-api',
      confidence: 0.9,
    })

    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Site URLs')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Site URLs'), {
      target: { value: 'https://a.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Identify step renders the detected platform in the platform input.
    await waitFor(() => {
      expect(screen.getByDisplayValue('new-api')).toBeInTheDocument()
    })
    // Back button only appears on non-source steps.
    expect(screen.getByRole('button', { name: 'Back' })).toBeInTheDocument()
  })

  it('shows a toast when advancing from identify with a missing platform', async () => {
    // Detection returns undetectable → platform stays empty.
    mockDetectMutate.mockResolvedValue({})

    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Site URLs')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Site URLs'), {
      target: { value: 'https://a.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for identify step (Back button appears).
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Back' })).toBeInTheDocument()
    })
    // Let detection settle so the platform input has its final value.
    await waitFor(() => {
      expect(mockDetectMutate).toHaveBeenCalledTimes(1)
    })

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(mockToastError).toHaveBeenCalledWith(
      expect.stringContaining('platform')
    )
  })

  it('renders the per-item reason on the done step for a failed item', async () => {
    mockDetectMutate.mockResolvedValue({
      platform: 'new-api',
      confidence: 0.9,
    })
    mockImportMutate.mockResolvedValue({
      imported: 0,
      skipped: 0,
      failed: 1,
      results: [
        {
          name: 'a.com',
          url: 'https://a.com',
          status: 'failed',
          reason: 'platform unsupported',
        },
      ],
    })

    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    // source → identify
    await waitFor(() => {
      expect(screen.getByLabelText('Site URLs')).toBeInTheDocument()
    })
    fireEvent.change(screen.getByLabelText('Site URLs'), {
      target: { value: 'https://a.com' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for detection to populate the platform.
    await waitFor(() => {
      expect(screen.getByDisplayValue('new-api')).toBeInTheDocument()
    })

    // identify → connect
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for connect step (Switch toggle appears).
    await waitFor(() => {
      expect(screen.getByRole('switch')).toBeInTheDocument()
    })
    // connect → routes
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for routes step (weight input renders as a spinbutton).
    await waitFor(() => {
      expect(screen.getByRole('spinbutton')).toBeInTheDocument()
    })
    // routes → done
    fireEvent.click(screen.getByRole('button', { name: 'Import' }))

    // Done step renders the raw backend reason (interpolated into the
    // i18n "Reason: {{reason}}" label).
    await waitFor(() => {
      expect(screen.getByText(/platform unsupported/)).toBeInTheDocument()
    })
  })

  it('opens the discard confirm dialog when closing with unsaved input', async () => {
    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Site URLs')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Site URLs'), {
      target: { value: 'https://a.com' },
    })

    // Trigger close via the dialog's close (X) button.
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))

    // The discard confirm dialog should appear with its title.
    await waitFor(() => {
      expect(screen.getByText('Discard this import?')).toBeInTheDocument()
    })
  })
})

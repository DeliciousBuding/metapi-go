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

import { ImportWizardDialog } from '../components/import-wizard-dialog'

const { mockDetectMutate, mockImportMutate, mockToastError } = vi.hoisted(
  () => ({
    mockDetectMutate: vi.fn(),
    mockImportMutate: vi.fn(),
    mockToastError: vi.fn(),
  })
)

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

  // Focus-first-invalid regression tests. The wizard marks the first
  // invalid field aria-invalid and moves focus to it when a step's
  // validation fails, then clears the flag once the user edits that field.
  // happy-dom click events do not move focus the way a real browser does,
  // so each test explicitly blurs the target field before triggering
  // validation — otherwise autoFocus (source) or a prior focus would leave
  // the field already focused and the re-focus assertion would be vacuous.

  it('focuses the source field and marks it aria-invalid when Next fires with no URLs', async () => {
    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    const sourceField = await screen.findByLabelText('Site URLs')
    sourceField.blur()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(mockToastError).toHaveBeenCalledTimes(1)
    await waitFor(() => {
      expect(document.activeElement).toBe(sourceField)
    })
    expect(sourceField).toHaveAttribute('aria-invalid', 'true')
  })

  it('focuses the first empty platform field on identify when a platform is missing', async () => {
    // Detection returns undetectable → platform stays empty.
    mockDetectMutate.mockResolvedValue({})

    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    const sourceField = await screen.findByLabelText('Site URLs')
    fireEvent.change(sourceField, { target: { value: 'https://a.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for identify step (Back button appears) and detection to settle
    // so the platform input holds its final value.
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Back' })).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(mockDetectMutate).toHaveBeenCalledTimes(1)
    })

    const platformField = screen.getByLabelText('Platform')
    platformField.blur()

    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    expect(mockToastError).toHaveBeenCalledWith(
      expect.stringContaining('platform')
    )
    await waitFor(() => {
      expect(document.activeElement).toBe(platformField)
    })
    expect(platformField).toHaveAttribute('aria-invalid', 'true')
  })

  it('focuses the first invalid weight field on submit and blocks the import call', async () => {
    mockDetectMutate.mockResolvedValue({
      platform: 'new-api',
      confidence: 0.9,
    })

    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    const sourceField = await screen.findByLabelText('Site URLs')
    fireEvent.change(sourceField, { target: { value: 'https://a.com' } })
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))

    // Wait for detection to populate the platform so identify can advance.
    await waitFor(() => {
      expect(screen.getByDisplayValue('new-api')).toBeInTheDocument()
    })

    // identify → connect
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(screen.getByRole('switch')).toBeInTheDocument()
    })

    // connect → routes
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    const weightField = await screen.findByLabelText('Weight')
    // Negative weight trips the `weight < 0` guard in handleSubmit.
    fireEvent.change(weightField, { target: { value: '-1' } })
    weightField.blur()

    fireEvent.click(screen.getByRole('button', { name: 'Import' }))

    expect(mockToastError).toHaveBeenCalledWith(
      expect.stringContaining('Weight')
    )
    // Validation failure must short-circuit before the import mutation fires.
    expect(mockImportMutate).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(document.activeElement).toBe(weightField)
    })
    expect(weightField).toHaveAttribute('aria-invalid', 'true')
  })

  it('clears aria-invalid on the source field once the user edits it', async () => {
    render(<ImportWizardDialog open onOpenChange={() => {}} />)

    const sourceField = await screen.findByLabelText('Site URLs')
    sourceField.blur()

    // Establish the invalid state first.
    fireEvent.click(screen.getByRole('button', { name: 'Next' }))
    await waitFor(() => {
      expect(sourceField).toHaveAttribute('aria-invalid', 'true')
    })

    // Editing the field must clear the flag immediately so an error never
    // outlives the input that caused it.
    fireEvent.change(sourceField, { target: { value: 'https://a.com' } })

    await waitFor(() => {
      expect(sourceField).not.toHaveAttribute('aria-invalid')
    })
  })
})

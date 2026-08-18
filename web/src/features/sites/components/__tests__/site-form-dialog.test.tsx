// Regression tests for the add/edit site form dialog. Guards the gap-1
// round-trip of `customHeadersOverrideRequestHeaders`, Zod submission
// errors, the create payload reaching `onCreated`, and the dirty-close
// confirm guard. Mocks only the sites api + toast; keeps the real
// RHF + Zod + dirty-close-hook code paths under test.
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

import type { Site } from '../../types'
import { SiteFormDialog } from '../site-form-dialog'

const { mockCreateMutate, mockUpdateMutate, mockDetectMutate, mockToastError } =
  vi.hoisted(() => ({
    mockCreateMutate: vi.fn(),
    mockUpdateMutate: vi.fn(),
    mockDetectMutate: vi.fn(),
    mockToastError: vi.fn(),
  }))

vi.mock('../../api', () => ({
  useCreateSite: () => ({ mutateAsync: mockCreateMutate, isPending: false }),
  useUpdateSite: () => ({ mutateAsync: mockUpdateMutate, isPending: false }),
  useDetectSite: () => ({ mutateAsync: mockDetectMutate, isPending: false }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: mockToastError,
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

beforeAll(() => {
  // base-ui Dialog / AlertDialog / Select need matchMedia under jsdom.
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
  mockCreateMutate.mockReset()
  mockUpdateMutate.mockReset()
  mockDetectMutate.mockReset()
  mockToastError.mockReset()
  // Default: detection returns nothing so the platform stays manual.
  mockDetectMutate.mockResolvedValue({})
})

afterEach(() => cleanup())

function typeField(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

describe('SiteFormDialog Zod submission errors', () => {
  it('renders the nameRequired error when submitting with an empty name', async () => {
    render(<SiteFormDialog open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Please enter a name.')).toBeInTheDocument()
    })
    // Validation failure must short-circuit before the create mutation fires.
    expect(mockCreateMutate).not.toHaveBeenCalled()
    expect(mockToastError).toHaveBeenCalledTimes(1)
  })
})

describe('SiteFormDialog create payload', () => {
  it('calls onCreated with the created site after a valid submit', async () => {
    const createdSite: Site = {
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    }
    mockCreateMutate.mockResolvedValue(createdSite)
    const onCreated = vi.fn()

    render(
      <SiteFormDialog
        open
        onOpenChange={vi.fn()}
        editingSite={null}
        onCreated={onCreated}
      />
    )

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'My Site')
    typeField('URL', 'https://example.com')

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    // The payload must carry the entered fields plus the default
    // `customHeadersOverrideRequestHeaders: false` (gap-1 round-trip
    // contract: create sends the schema's boolean, never undefined).
    const payload = mockCreateMutate.mock.calls[0]?.[0]
    expect(payload).toMatchObject({
      name: 'My Site',
      url: 'https://example.com',
      customHeadersOverrideRequestHeaders: false,
    })
    expect(onCreated).toHaveBeenCalledWith(createdSite)
  })
})

describe('SiteFormDialog post-refresh probe latency threshold (gap-11)', () => {
  it('renders the latency threshold field when probe is enabled and round-trips the value into the payload', async () => {
    // A successful create resolves with a minimal site object; only the
    // payload contract is asserted, not the created echo.
    mockCreateMutate.mockResolvedValue({
      id: 99,
      name: 'Probe site',
      url: 'https://probe.example',
      platform: '',
      status: 'active',
    })

    render(<SiteFormDialog open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    // Fill the required fields so a valid submit can fire.
    typeField('Name', 'Probe site')
    typeField('URL', 'https://probe.example')

    // The latency threshold field must NOT render while the probe is off.
    expect(
      screen.queryByLabelText('Probe latency threshold (ms)')
    ).not.toBeInTheDocument()

    // Toggle the post-refresh probe switch on; the conditional block
    // (model + scope + latency threshold) must now render.
    fireEvent.click(screen.getByRole('switch', { name: 'Post-refresh probe' }))

    const latencyField = await screen.findByLabelText(
      'Probe latency threshold (ms)'
    )
    expect(latencyField).toBeInTheDocument()

    // Enter a threshold; the numeric onChange (valueAsNumber) must
    // round-trip the entered value into the submit payload.
    fireEvent.change(latencyField, { target: { value: '1500' } })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    const payload = mockCreateMutate.mock.calls[0]?.[0]
    expect(payload).toMatchObject({
      name: 'Probe site',
      url: 'https://probe.example',
      postRefreshProbeEnabled: true,
      postRefreshProbeLatencyThresholdMs: 1500,
    })
  })
})

describe('SiteFormDialog edit round-trip (gap-1)', () => {
  it('checks the override-headers switch when editing a site with it enabled', async () => {
    const editingSite: Site = {
      id: 7,
      name: 'Existing site',
      url: 'https://existing.example',
      platform: 'openai',
      status: 'active',
      customHeaders: '{"X-Test":"1"}',
      customHeadersOverrideRequestHeaders: true,
    }

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={editingSite} />
    )

    const overrideSwitch = await screen.findByRole('switch', {
      name: 'Override request headers',
    })

    await waitFor(() => {
      expect(overrideSwitch).toHaveAttribute('aria-checked', 'true')
    })
  })
})

describe('SiteFormDialog dirty-close guard', () => {
  it('opens the discard confirm when closing with unsaved edits', async () => {
    render(<SiteFormDialog open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'dirty value')

    // Trigger close via the dialog's close (X) button. The dirty-close
    // guard must intercept and show the confirm instead of discarding.
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))

    await waitFor(() => {
      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument()
    })
  })
})

// Focused vitest for the apiEndpoints structured editor in the add/edit
// site dialog (issue #861): structured-row interaction (add / reorder /
// toggle), advanced-mode textarea compatibility, untouched-preserve
// semantics and validation-error rendering. Mocks only the sites api +
// toast; keeps the real RHF + Zod + dirty-close-guard + i18n code paths
// under test (same pattern as site-form-dialog.test.tsx).
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

import type { Site, SiteFormPayload } from '../../types'
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
  // jsdom does not implement scrollIntoView; FormValidationFocus calls it
  // after a failed submit to scroll the first invalid field into view.
  Element.prototype.scrollIntoView = vi.fn()
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

const EDITING_SITE: Site = {
  id: 7,
  name: 'Existing site',
  url: 'https://existing.example',
  platform: 'openai',
  status: 'active',
  apiEndpoints: [
    { id: 1, url: 'https://ep1.example', enabled: false, sortOrder: 5 },
    { id: 2, url: 'https://ep2.example' },
  ],
  tags: ['prod'],
}

function typeField(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

/** Opens the collapsed advanced textarea mode. */
function openAdvanced() {
  fireEvent.click(screen.getByRole('button', { name: /Advanced/ }))
}

/** Renders the create dialog with the mandatory fields filled. */
async function renderCreateDialog(): Promise<void> {
  render(<SiteFormDialog open onOpenChange={vi.fn()} editingSite={null} />)
  await waitFor(() => {
    expect(screen.getByLabelText('Name')).toBeInTheDocument()
  })
  typeField('Name', 'My Site')
  typeField('URL', 'https://example.com')
}

function lastCreatePayload(): SiteFormPayload {
  return mockCreateMutate.mock.calls[0]?.[0] as SiteFormPayload
}

describe('SiteFormDialog apiEndpoints structured editor interaction', () => {
  it('adds rows and sends url/enabled/positional sortOrder on create', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    expect(
      screen.getByText(
        'No API endpoints yet; when empty the primary site URL is used.'
      )
    ).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /Add API endpoint/ }))
    fireEvent.change(screen.getByLabelText('Endpoint URL 1'), {
      target: { value: 'https://a.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Add API endpoint/ }))
    fireEvent.change(screen.getByLabelText('Endpoint URL 2'), {
      target: { value: 'https://b.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })
    expect(lastCreatePayload()).toMatchObject({
      apiEndpoints: [
        { url: 'https://a.example', enabled: true, sortOrder: 0 },
        { url: 'https://b.example', enabled: true, sortOrder: 1 },
      ],
    })
  })

  it('serializes enabled toggles and reorder operations (list order = polling order)', async () => {
    mockUpdateMutate.mockResolvedValue({ ...EDITING_SITE, name: 'Renamed' })

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={EDITING_SITE} />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Endpoint URL 1')).toBeInTheDocument()
    })

    // Endpoint 1 starts disabled (enabled: false) — enable it, then move
    // it down so the list order flips.
    const switch1 = screen.getByLabelText('Enable endpoint 1')
    expect(
      switch1.closest('label')?.getAttribute('aria-checked') ??
        switch1.getAttribute('aria-checked') ??
        'false'
    ).toBe('false')
    fireEvent.click(switch1)
    fireEvent.click(screen.getByLabelText('Move endpoint 1 down'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
    })
    const updateArgs = mockUpdateMutate.mock.calls[0]?.[0] as {
      id: number
      payload: SiteFormPayload
    }
    // After the swap the list is [ep2, ep1] with positional sortOrder.
    expect(updateArgs.payload.apiEndpoints).toEqual([
      { url: 'https://ep2.example', enabled: true, sortOrder: 0 },
      { url: 'https://ep1.example', enabled: true, sortOrder: 1 },
    ])
  })

  it('sends an empty list when all rows are removed on edit', async () => {
    mockUpdateMutate.mockResolvedValue({ ...EDITING_SITE })

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={EDITING_SITE} />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Endpoint URL 1')).toBeInTheDocument()
    })

    // Delete both rows. Row labels re-number after each removal, so the
    // remaining row is "endpoint 1" again.
    fireEvent.click(screen.getByLabelText('Delete endpoint 1'))
    fireEvent.click(screen.getByLabelText('Delete endpoint 1'))
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
    })
    const updateArgs = mockUpdateMutate.mock.calls[0]?.[0] as {
      payload: SiteFormPayload
    }
    expect(updateArgs.payload.apiEndpoints).toEqual([])
  })

  it('keeps the advanced textarea compatible with plain-URL lines', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 44,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    openAdvanced()
    fireEvent.change(screen.getByLabelText('API endpoints JSON text'), {
      target: { value: 'https://a.example\n\nhttps://b.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })
    expect(lastCreatePayload()).toMatchObject({
      apiEndpoints: [
        { url: 'https://a.example', enabled: true, sortOrder: 0 },
        { url: 'https://b.example', enabled: true, sortOrder: 1 },
      ],
    })
  })
})

describe('SiteFormDialog apiEndpoints untouched-preserve semantics', () => {
  it('passes the original endpoints through on name-only edits', async () => {
    mockUpdateMutate.mockResolvedValue({ ...EDITING_SITE, name: 'Renamed' })

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={EDITING_SITE} />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'Renamed')
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
    })
    const updateArgs = mockUpdateMutate.mock.calls[0]?.[0] as {
      id: number
      payload: SiteFormPayload
    }
    expect(updateArgs.id).toBe(7)
    // Untouched preserve: the original enabled/sortOrder survive the save.
    expect(updateArgs.payload.apiEndpoints).toEqual([
      { url: 'https://ep1.example', enabled: false, sortOrder: 5 },
      { url: 'https://ep2.example', enabled: true, sortOrder: 0 },
    ])
  })

  it('marks the form dirty for the dirty-close guard when only the editor changes', async () => {
    const onOpenChange = vi.fn()
    render(
      <SiteFormDialog
        open
        onOpenChange={onOpenChange}
        editingSite={EDITING_SITE}
      />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Enable endpoint 1')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByLabelText('Enable endpoint 1'))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument()
    })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})

describe('SiteFormDialog apiEndpoints validation errors', () => {
  it('renders the invalid-URL error and short-circuits the create mutation', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 45,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.click(screen.getByRole('button', { name: /Add API endpoint/ }))
    fireEvent.change(screen.getByLabelText('Endpoint URL 1'), {
      target: { value: 'not a url' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(
        screen.getByText(
          'Endpoint URLs must be valid http(s) URLs; cloud metadata and link-local targets are not allowed.'
        )
      ).toBeInTheDocument()
    })
    expect(mockCreateMutate).not.toHaveBeenCalled()
    expect(mockToastError).toHaveBeenCalledTimes(1)
  })

  it('renders the duplicate-URL error for normalized duplicates', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 46,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.click(screen.getByRole('button', { name: /Add API endpoint/ }))
    fireEvent.change(screen.getByLabelText('Endpoint URL 1'), {
      target: { value: 'https://dup.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: /Add API endpoint/ }))
    fireEvent.change(screen.getByLabelText('Endpoint URL 2'), {
      target: { value: 'https://dup.example/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(
        screen.getByText('The endpoint list contains duplicate URLs.')
      ).toBeInTheDocument()
    })
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('renders the invalid-entry error for malformed JSON-object lines in advanced mode', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 47,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    openAdvanced()
    fireEvent.change(screen.getByLabelText('API endpoints JSON text'), {
      target: { value: '{"enabled":true}' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(
        screen.getByText(
          'Each endpoint line must be a plain URL, or a JSON object with a url string and an optional boolean "enabled" and integer "sortOrder".'
        )
      ).toBeInTheDocument()
    })
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('shows the cooldown and failure status on the rows of the edited site', async () => {
    const siteWithStatus: Site = {
      ...EDITING_SITE,
      apiEndpoints: [
        {
          id: 1,
          url: 'https://ep1.example',
          enabled: false,
          sortOrder: 5,
          cooldownUntil: new Date(Date.now() + 3600_000).toISOString(),
          lastFailureReason: 'upstream 421 mule_route_not_found',
        },
        { id: 2, url: 'https://ep2.example' },
      ],
    }
    mockUpdateMutate.mockResolvedValue({ ...siteWithStatus })

    render(
      <SiteFormDialog
        open
        onOpenChange={vi.fn()}
        editingSite={siteWithStatus}
      />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('Endpoint URL 1')).toBeInTheDocument()
    })

    // Row 1 carries the live cooldown + failure signal; row 2 has neither,
    // so row 2 renders no status span at all.
    const statusText =
      screen.getByText('Order #1').parentElement?.textContent ?? ''
    expect(statusText).toContain('Cooling down')
    expect(statusText).toContain('upstream 421 mule_route_not_found')
    const statusText2 =
      screen.getByText('Order #2').parentElement?.textContent ?? ''
    expect(statusText2).not.toContain('Fail')
  })
})

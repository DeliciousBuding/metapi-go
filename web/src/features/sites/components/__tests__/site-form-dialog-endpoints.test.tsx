// Focused vitest for the apiEndpoints editor in the add/edit site dialog
// (issue #861): editor interaction, untouched-preserve semantics and
// validation-error rendering. Mocks only the sites api + toast; keeps the
// real RHF + Zod + dirty-close-guard + i18n code paths under test (same
// pattern as site-form-dialog.test.tsx).
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

describe('SiteFormDialog apiEndpoints editor interaction', () => {
  it('sends parsed plain-URL lines (enabled true, positional sortOrder) on create', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.change(screen.getByLabelText('API endpoints'), {
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

  it('sends JSON-object lines with explicit enabled/sortOrder on create', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 43,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: {
        value: '{"url":"https://b.example","enabled":false,"sortOrder":7}',
      },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })
    expect(lastCreatePayload().apiEndpoints).toEqual([
      { url: 'https://b.example', enabled: false, sortOrder: 7 },
    ])
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

  it('uses the edited list once the textarea is touched', async () => {
    mockUpdateMutate.mockResolvedValue({ ...EDITING_SITE, name: 'Renamed' })

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={EDITING_SITE} />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('API endpoints')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: { value: 'https://ep1.example\nhttps://new.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
    })
    const updateArgs = mockUpdateMutate.mock.calls[0]?.[0] as {
      payload: SiteFormPayload
    }
    expect(updateArgs.payload.apiEndpoints).toEqual([
      { url: 'https://ep1.example', enabled: true, sortOrder: 0 },
      { url: 'https://new.example', enabled: true, sortOrder: 1 },
    ])
  })

  it('sends an empty list when the editor is cleared on edit', async () => {
    mockUpdateMutate.mockResolvedValue({ ...EDITING_SITE })

    render(
      <SiteFormDialog open onOpenChange={vi.fn()} editingSite={EDITING_SITE} />
    )
    await waitFor(() => {
      expect(screen.getByLabelText('API endpoints')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: { value: '   \n\n' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateMutate).toHaveBeenCalledTimes(1)
    })
    const updateArgs = mockUpdateMutate.mock.calls[0]?.[0] as {
      payload: SiteFormPayload
    }
    expect(updateArgs.payload.apiEndpoints).toEqual([])
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
      expect(screen.getByLabelText('API endpoints')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: { value: 'https://adjacent.example' },
    })
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
      id: 44,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: { value: 'https://ok.example\nnot a url' },
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
      id: 45,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.change(screen.getByLabelText('API endpoints'), {
      target: { value: 'https://dup.example\nhttps://dup.example/' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(
        screen.getByText('The endpoint list contains duplicate URLs.')
      ).toBeInTheDocument()
    })
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })

  it('renders the invalid-entry error for malformed JSON-object lines', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 46,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    })
    await renderCreateDialog()

    fireEvent.change(screen.getByLabelText('API endpoints'), {
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
})

// Focus-first-invalid verification (audit P2 #5 closeout): the site form
// dialog renders inside <Form>, so the design system's FormValidationFocus
// must move focus to the first invalid control (the name input) when an
// empty form is submitted.

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

import { SiteFormSheet } from '../components/site-form-sheet'

const { mockToastError } = vi.hoisted(() => ({ mockToastError: vi.fn() }))

vi.mock('@/lib/toast', () => ({
  toast: {
    error: mockToastError,
    success: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
    message: vi.fn(),
    loading: vi.fn(),
  },
}))

vi.mock('../api', () => ({
  useCreateSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDetectSite: () => ({
    mutateAsync: vi.fn().mockResolvedValue({}),
    isPending: false,
  }),
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
  // jsdom does not implement scrollIntoView; FormValidationFocus calls it
  // before focusing the invalid control.
  Element.prototype.scrollIntoView = vi.fn()
})

beforeEach(() => {
  mockToastError.mockReset()
})

afterEach(() => cleanup())

describe('SiteFormSheet focus-first-invalid', () => {
  it('focuses the first invalid field when submitting an empty form', async () => {
    render(<SiteFormSheet open onOpenChange={() => {}} editingSite={null} />)

    const submitButton = await screen.findByRole('button', { name: 'Create' })
    fireEvent.click(submitButton)

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledTimes(1)
    })

    // Name is the first field in the site form; it must be marked invalid
    // and receive focus.
    await waitFor(() => {
      const invalidControl = document.querySelector<HTMLElement>(
        '[aria-invalid="true"]'
      )
      expect(invalidControl).not.toBeNull()
      expect(document.activeElement).toBe(invalidControl)
    })
    expect(screen.getByLabelText('Name')).toHaveAttribute(
      'aria-invalid',
      'true'
    )
  })
})

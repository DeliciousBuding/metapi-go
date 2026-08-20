// Regression test for the route form dialog's dirty-close guard. The
// explicit Cancel button must route through the same guard as Esc/X and
// never silently discard unsaved input (issue #889). Mocks only the route
// api + toasts; keeps the real RHF + Zod + dirty-close-hook paths under test.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { RouteFormDialog } from '../route-form-dialog'

vi.mock('../../api', () => ({
  useCreateRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateRoute: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useBatchAddChannels: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRebuildRoutes: () => ({ mutate: vi.fn(), isPending: false }),
  resolveCreatedRouteId: () => 1,
}))

vi.mock('../route-completion-toast', () => ({
  showRouteCompletionToast: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

beforeAll(() => {
  // base-ui Sheet / AlertDialog / Select need matchMedia under jsdom.
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

afterEach(() => cleanup())

describe('RouteFormDialog dirty-close guard', () => {
  it('opens the discard confirm when Cancel is clicked with unsaved edits', async () => {
    const onOpenChange = vi.fn()
    render(
      <RouteFormDialog
        open
        onOpenChange={onOpenChange}
        mode='create'
        route={null}
        availableRoutes={[]}
        accountOptions={[]}
      />
    )

    const displayIconField = await screen.findByLabelText(
      'Display icon (optional)'
    )
    fireEvent.change(displayIconField, { target: { value: 'rocket' } })

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument()
    })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})

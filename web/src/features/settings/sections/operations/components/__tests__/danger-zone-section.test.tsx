// Type-to-confirm gate for the irreversible factory reset. The 3-second
// countdown stops misclicks but not "read it yet didn't absorb it"; the confirm
// button must also stay disabled until the operator types the word RESET
// (W19-T3 N2 / T1 §0.2, GitHub-style hard gate).
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
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

import { DangerZoneSection } from '../danger-zone-section'

const { mockFactoryReset, mockToastSuccess, mockToastError } = vi.hoisted(
  () => ({
    mockFactoryReset: vi.fn(),
    mockToastSuccess: vi.fn(),
    mockToastError: vi.fn(),
  })
)

vi.mock('@/lib/api', () => ({
  api: { factoryReset: mockFactoryReset },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

beforeAll(() => {
  // base-ui Dialog queries matchMedia under jsdom.
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
  mockFactoryReset.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  // Never resolve so the mutation's onSuccess (window.location.reload) never
  // runs inside the test.
  mockFactoryReset.mockReturnValue(new Promise(() => {}))
})

afterEach(() => cleanup())

function renderDangerZone() {
  const queryClient = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <DangerZoneSection />
    </QueryClientProvider>
  )
}

async function openDialogAndType(word: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Factory reset' }))
  const input = await screen.findByPlaceholderText('RESET')
  fireEvent.change(input, { target: { value: word } })
  // The confirm action only reads "Wipe everything" once the countdown elapses.
  return screen.findByRole(
    'button',
    { name: 'Wipe everything' },
    { timeout: 6000 }
  )
}

describe('DangerZoneSection — factory reset type-to-confirm', () => {
  it('enables the wipe action only after typing RESET', async () => {
    renderDangerZone()

    const confirmButton = await openDialogAndType('RESET')
    expect(confirmButton).toBeEnabled()

    fireEvent.click(confirmButton)
    await waitFor(() => {
      expect(mockFactoryReset).toHaveBeenCalledTimes(1)
    })
  })

  it('keeps the wipe action disabled for a wrong confirm word', async () => {
    renderDangerZone()

    const confirmButton = await openDialogAndType('reset')
    expect(confirmButton).toBeDisabled()
    expect(mockFactoryReset).not.toHaveBeenCalled()
  })
})

// Regression test: the price-compare empty state was a dead end. It must
// offer a "Manage accounts" CTA pointing at /accounts, mirroring the models
// page empty-state exit (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
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

import { PriceComparePage } from '../price-compare-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => ({}),
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

vi.mock('@/components/common/query-error-banner', () => ({
  QueryErrorBanner: () => null,
}))

vi.mock('../../api', () => ({
  usePriceCompare: () => ({
    data: [],
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

vi.mock('../components/price-grade-badge', () => ({
  PriceGradeBadge: () => null,
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
  testState.navigate.mockReset()
})

afterEach(() => cleanup())

describe('PriceComparePage empty-state CTA', () => {
  it('offers a Manage accounts CTA that navigates to /accounts', () => {
    render(<PriceComparePage />)

    const cta = screen.getByRole('button', { name: 'Manage accounts' })
    expect(cta).toBeInTheDocument()

    fireEvent.click(cta)
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith({ to: '/accounts' })
  })
})

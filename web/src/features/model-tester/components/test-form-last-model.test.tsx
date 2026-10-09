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

import i18n from '@/i18n/config'

import { TestForm } from './test-form'

const catalog = vi.hoisted(() => ({
  models: [{ name: 'model-a' }, { name: 'model-b' }],
}))

vi.mock('@/features/models', () => ({
  useModels: () => ({
    data: catalog.models,
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

vi.mock('@/features/channels', () => ({
  useChannels: () => ({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  })
})

beforeEach(async () => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
  Element.prototype.scrollIntoView = vi.fn()

  window.localStorage.clear()
  catalog.models = [{ name: 'model-a' }, { name: 'model-b' }]
  await i18n.changeLanguage('en')
})

afterEach(() => cleanup())

function showForm(defaultModel?: string) {
  return render(
    <TestForm
      isRunning={false}
      defaultModel={defaultModel}
      onSubmit={vi.fn()}
      onStop={vi.fn()}
    />
  )
}

const savedKey = 'metapi-model-tester-last-model'

describe('model tester selection', () => {
  it('restores a stored model only while it exists in the live catalog', async () => {
    window.localStorage.setItem(savedKey, 'model-b')
    showForm()
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'model-b'
    )
  })

  it('ignores a model removed from the catalog', () => {
    window.localStorage.setItem(savedKey, 'retired-model')
    showForm()
    expect(
      screen.getByRole('combobox', { name: 'Model' })
    ).not.toHaveTextContent('retired-model')
  })

  it('lets an explicit model deep link outrank the saved choice', () => {
    window.localStorage.setItem(savedKey, 'model-b')
    showForm('model-a')
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'model-a'
    )
  })

  it('does not substitute a saved model for an invalid explicit deep link', () => {
    window.localStorage.setItem(savedKey, 'model-b')
    showForm('retired-model')
    expect(
      screen.getByRole('combobox', { name: 'Model' })
    ).not.toHaveTextContent('model-b')
  })

  it('remembers a user choice without resetting it on a catalog refetch', async () => {
    const { rerender, unmount } = showForm('model-a')
    fireEvent.click(screen.getByRole('combobox', { name: 'Model' }))
    const option = await screen.findByRole('option', { name: 'model-b' })
    fireEvent.pointerDown(option)
    fireEvent.click(option)

    await waitFor(() =>
      expect(window.localStorage.getItem(savedKey)).toBe('model-b')
    )
    catalog.models = [...catalog.models]
    rerender(
      <TestForm
        isRunning={false}
        defaultModel='model-a'
        onSubmit={vi.fn()}
        onStop={vi.fn()}
      />
    )
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'model-b'
    )

    unmount()
    showForm()
    expect(screen.getByRole('combobox', { name: 'Model' })).toHaveTextContent(
      'model-b'
    )
  })
})

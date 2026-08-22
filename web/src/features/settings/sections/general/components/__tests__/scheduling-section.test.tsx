// Partial-failure honesty for the settings "Trigger check-in now" action.
//
// POST /api/checkin/trigger answers 200 with `success: (failed == 0)`. The
// section previously ignored the envelope entirely and toasted success even
// when accounts failed. The trigger must report partial outcomes with the
// real counts and only celebrate when nothing failed.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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

import { SchedulingSection } from '../scheduling-section'

const testState = vi.hoisted(() => ({
  triggerCheckinAll: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  toastWarning: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeSettings: vi.fn().mockResolvedValue({}),
    triggerCheckinAll: (...args: unknown[]) =>
      testState.triggerCheckinAll(...args),
    getSettingsMigrationPreview: vi
      .fn()
      .mockResolvedValue({ success: true, pending: 0 }),
    applySettingsMigration: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: testState.toastSuccess,
    error: testState.toastError,
    warning: testState.toastWarning,
    info: vi.fn(),
  },
}))

// The router primitives are irrelevant to the trigger behaviour.
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: React.ReactNode }) => <a>{props.children}</a>,
}))

// Keep the harness focused on the mutation: heavy form chrome is stubbed.
vi.mock('../../../../components/form-navigation-guard', () => ({
  FormNavigationGuard: () => null,
}))

vi.mock('../../../../components/schedule-editor', () => ({
  ScheduleEditor: () => <div data-testid='schedule-editor-stub' />,
}))

vi.mock('../../../../components/settings-form-actions', () => ({
  SettingsFormActions: () => null,
}))

beforeAll(() => {
  // base-ui primitives query matchMedia under jsdom.
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
  testState.triggerCheckinAll.mockReset()
  testState.toastSuccess.mockReset()
  testState.toastError.mockReset()
  testState.toastWarning.mockReset()
})

afterEach(() => cleanup())

function renderSchedulingSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SchedulingSection />
    </QueryClientProvider>
  )
}

describe('SchedulingSection — trigger check-in honesty', () => {
  it('warns with the real counts when some accounts failed', async () => {
    testState.triggerCheckinAll.mockResolvedValue({
      success: false,
      queued: false,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 3, success: 1, failed: 2, skipped: 0 },
    })

    renderSchedulingSection()

    fireEvent.click(
      await screen.findByRole('button', {
        name: /Trigger check-in now|立即签到/,
      })
    )

    await waitFor(() => {
      expect(testState.toastWarning).toHaveBeenCalledTimes(1)
    })
    const [title, options] = testState.toastWarning.mock.calls[0] ?? []
    expect(String(title)).toMatch(/partially failed|部分失败/)
    expect(String((options as { description?: string })?.description)).toMatch(
      /1|2|3/
    )
    expect(testState.toastSuccess).not.toHaveBeenCalled()
  })

  it('toasts success only when every account succeeded', async () => {
    testState.triggerCheckinAll.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 2, success: 2, failed: 0, skipped: 0 },
    })

    renderSchedulingSection()

    fireEvent.click(
      await screen.findByRole('button', {
        name: /Trigger check-in now|立即签到/,
      })
    )

    await waitFor(() => {
      expect(testState.toastSuccess).toHaveBeenCalledTimes(1)
    })
    expect(testState.toastWarning).not.toHaveBeenCalled()
    expect(testState.toastError).not.toHaveBeenCalled()
  })
})

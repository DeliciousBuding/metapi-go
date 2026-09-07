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
import {
  ROUTE_REBUILD_STORAGE_KEY,
  rememberRouteRebuild,
} from '@/features/token-routes/lib/route-rebuild-reference'

import { AllowlistSection } from '../allowlist-section'

const state = vi.hoisted(() => ({
  getRuntimeSettings: vi.fn(),
  updateRuntimeSettings: vi.fn(),
  rebuildRoutes: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeSettings: state.getRuntimeSettings,
    updateRuntimeSettings: state.updateRuntimeSettings,
    rebuildRoutes: state.rebuildRoutes,
    getBrandList: vi.fn().mockResolvedValue({ brands: ['openai'] }),
    getModelTokenCandidates: vi.fn().mockResolvedValue({ models: {} }),
    getSettingsMigrationPreview: vi
      .fn()
      .mockResolvedValue({ success: true, pending: 0 }),
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: {
    success: state.success,
    error: state.error,
    warning: state.warning,
    info: state.info,
  },
}))
vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children?: React.ReactNode }) => <a>{props.children}</a>,
}))
vi.mock('../../../../components/form-navigation-guard', () => ({
  FormNavigationGuard: () => null,
}))
vi.mock('../../../../components/settings-form-actions', () => ({
  SettingsFormActions: () => null,
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation(() => ({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
    })),
  })
})
beforeEach(() => {
  vi.clearAllMocks()
  rememberRouteRebuild(null)
  const runtime = {
    globalAllowedModels: [] as string[],
    globalBlockedBrands: [] as string[],
  }
  state.getRuntimeSettings.mockImplementation(async () => ({ ...runtime }))
  state.updateRuntimeSettings.mockImplementation(
    async (body: Partial<typeof runtime>) => {
      Object.assign(runtime, body)
      return { success: true }
    }
  )
  state.rebuildRoutes.mockResolvedValue({
    success: true,
    queued: true,
    taskId: 'allowlist-rebuild',
    jobId: 'allowlist-rebuild',
    status: 'pending',
  })
})
afterEach(() => {
  cleanup()
  rememberRouteRebuild(null)
})
function renderSection() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <AllowlistSection />
    </QueryClientProvider>
  )
}

describe('AllowlistSection background rebuild tracking', () => {
  it('stores the acknowledged task instead of treating a queue response as completed routing', async () => {
    renderSection()
    fireEvent.click(await screen.findByRole('switch'))
    await waitFor(() =>
      expect(state.rebuildRoutes).toHaveBeenCalledWith(false, false)
    )
    await waitFor(() =>
      expect(
        JSON.parse(
          window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY) ?? 'null'
        )
      ).toEqual({ taskId: 'allowlist-rebuild', refreshModels: false })
    )
    expect(state.updateRuntimeSettings).toHaveBeenCalledWith({
      globalBlockedBrands: ['openai'],
    })
  })
  it('does not report that the setting failed when only the rebuild acknowledgement was lost', async () => {
    state.rebuildRoutes.mockRejectedValue(new Error('response lost'))
    renderSection()
    fireEvent.click(await screen.findByRole('switch'))
    await waitFor(() => expect(state.warning).toHaveBeenCalled())
    await waitFor(() => expect(state.success).toHaveBeenCalled())
    expect(state.error).not.toHaveBeenCalled()
    expect(window.localStorage.getItem(ROUTE_REBUILD_STORAGE_KEY)).toBeNull()
  })
})

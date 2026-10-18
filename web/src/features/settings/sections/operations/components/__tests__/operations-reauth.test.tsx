// Behavior test for sensitive operations-section actions (#1034).
//
// Connection tests, database migration, usage purge and factory reset are
// all gated by the backend behind master-token re-confirmation. Each UI keeps
// its existing confirmation flow and only adds the token dialog when the
// server demands it, replaying the exact same request once.

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

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'

import { DangerZoneSection } from '../danger-zone-section'
import { DatabaseMigrationSection } from '../database-migration-section'
import { DatabaseSection } from '../database-section'
import { MaintenanceSection } from '../maintenance-section'

const {
  mockGetRuntimeDatabaseConfig,
  mockTestConnection,
  mockStartMigration,
  mockGetTask,
  mockClearRuntimeCache,
  mockClearUsageData,
  mockFactoryReset,
} = vi.hoisted(() => ({
  mockGetRuntimeDatabaseConfig: vi.fn(),
  mockTestConnection: vi.fn(),
  mockStartMigration: vi.fn(),
  mockGetTask: vi.fn(),
  mockClearRuntimeCache: vi.fn(),
  mockClearUsageData: vi.fn(),
  mockFactoryReset: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeDatabaseConfig: mockGetRuntimeDatabaseConfig,
    testExternalDatabaseConnection: mockTestConnection,
    startDatabaseMigration: mockStartMigration,
    getTask: mockGetTask,
    clearRuntimeCache: mockClearRuntimeCache,
    clearUsageData: mockClearUsageData,
    factoryReset: mockFactoryReset,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
}))

// FormNavigationGuard (useBlocker) and MaintenanceSection's Link are the only
// router consumers in this tree; stub both so no router is required.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', reset: vi.fn(), proceed: vi.fn() }),
  Link: (props: { children?: React.ReactNode; className?: string }) => (
    <a href='#' className={props.className}>
      {props.children}
    </a>
  ),
}))

const runtimeConfig = {
  active: { dialect: 'sqlite', connection: './data/metapi.db', ssl: false },
  saved: null,
  restartRequired: false,
}

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
  mockGetRuntimeDatabaseConfig.mockReset()
  mockTestConnection.mockReset()
  mockStartMigration.mockReset()
  mockGetTask.mockReset()
  mockClearRuntimeCache.mockReset()
  mockClearUsageData.mockReset()
  mockFactoryReset.mockReset()
  mockGetRuntimeDatabaseConfig.mockResolvedValue(runtimeConfig)
  mockClearRuntimeCache.mockResolvedValue({ success: true })
  mockClearUsageData.mockResolvedValue({ success: true })
})

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

function renderWithProviders(node: React.ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ReauthProvider>{node}</ReauthProvider>
    </QueryClientProvider>
  )
}

function clickLastButtonNamed(name: string) {
  const buttons = screen.getAllByRole('button', { name })
  const button = buttons.at(-1)
  if (!button) throw new Error(`Button "${name}" not found`)
  fireEvent.click(button)
}

async function confirmReauth(token: string) {
  const input = await screen.findByLabelText(text('reauth.tokenLabel'))
  fireEvent.change(input, { target: { value: token } })
  fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))
}

describe('database connection test — master-token replay', () => {
  it('replays the DSN probe with the token', async () => {
    mockTestConnection
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true })
    renderWithProviders(<DatabaseSection />)

    fireEvent.change(await screen.findByLabelText('Connection string'), {
      target: { value: 'postgres://user:pass@host:5432/db' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))

    await waitFor(() => expect(mockTestConnection).toHaveBeenCalledTimes(1))
    expect(mockTestConnection.mock.calls[0]).toHaveLength(1)

    await confirmReauth('master-token')

    await waitFor(() => expect(mockTestConnection).toHaveBeenCalledTimes(2))
    expect(mockTestConnection).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        connectionString: 'postgres://user:pass@host:5432/db',
      }),
      'master-token'
    )
  })
})

describe('database migration — master-token replay', () => {
  it('keeps the destructive confirm and replays the migration once with the token', async () => {
    mockStartMigration
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true, message: 'queued', taskId: 't1' })
    mockGetTask.mockReturnValue(new Promise(() => {}))
    renderWithProviders(<DatabaseMigrationSection />)

    fireEvent.change(await screen.findByLabelText('Connection string'), {
      target: { value: 'postgres://user:pass@host:5432/db' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Start migration' }))
    await screen.findByText('Confirm data migration')
    expect(mockStartMigration).not.toHaveBeenCalled()

    clickLastButtonNamed('Start migration')
    await waitFor(() => expect(mockStartMigration).toHaveBeenCalledTimes(1))
    expect(mockStartMigration.mock.calls[0]).toHaveLength(1)

    await confirmReauth('master-token')

    await waitFor(() => expect(mockStartMigration).toHaveBeenCalledTimes(2))
    expect(mockStartMigration).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        dialect: 'postgres',
        overwrite: true,
      }),
      'master-token'
    )
  })
})

describe('maintenance clear-usage — master-token replay', () => {
  it('replays the purge once with the token', async () => {
    mockClearUsageData
      .mockRejectedValueOnce(reauthError())
      .mockResolvedValueOnce({ success: true })
    renderWithProviders(<MaintenanceSection />)

    fireEvent.click(screen.getByRole('button', { name: 'Clear usage logs' }))
    clickLastButtonNamed('Clear usage logs')

    await waitFor(() => expect(mockClearUsageData).toHaveBeenCalledTimes(1))
    expect(mockClearUsageData.mock.calls[0]).toHaveLength(0)

    await confirmReauth('master-token')

    await waitFor(() => expect(mockClearUsageData).toHaveBeenCalledTimes(2))
    expect(mockClearUsageData).toHaveBeenNthCalledWith(2, 'master-token')
  })
})

describe('factory reset — master-token replay', () => {
  it('adds the token step after the type-to-confirm gate and replays once', async () => {
    mockFactoryReset
      .mockRejectedValueOnce(reauthError())
      // Never resolve so the reset success path (window reload) stays out of
      // the test; the assertion is the replay request itself.
      .mockReturnValueOnce(new Promise(() => {}))
    renderWithProviders(<DangerZoneSection />)

    fireEvent.click(screen.getByRole('button', { name: 'Factory reset' }))
    fireEvent.change(await screen.findByPlaceholderText('RESET'), {
      target: { value: 'RESET' },
    })
    fireEvent.click(
      await screen.findByRole(
        'button',
        { name: 'Wipe everything' },
        { timeout: 6000 }
      )
    )

    await waitFor(() => expect(mockFactoryReset).toHaveBeenCalledTimes(1))
    expect(mockFactoryReset.mock.calls[0]).toHaveLength(0)

    await confirmReauth('master-token')

    await waitFor(() => expect(mockFactoryReset).toHaveBeenCalledTimes(2))
    expect(mockFactoryReset).toHaveBeenNthCalledWith(2, 'master-token')
  })
})

// Behavior test for the database migration section.
//
// Locks the destructive-migration flow: target defaults to the dialect away
// from the live runtime, the connection string is required before testing or
// migrating, the start button opens the destructive confirm dialog, and after
// the backend accepts the job (202 + taskId) the task is polled through
// api.getTask until a terminal status surfaces (success toast + row summary,
// or error toast). The same-target 400 rejection is asserted to surface the
// localized error instead of the raw server message.

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

import { DatabaseMigrationSection } from '../database-migration-section'

const {
  mockGetConfig,
  mockTestConnection,
  mockStartMigration,
  mockGetTask,
  mockToastSuccess,
  mockToastError,
  mockToastInfo,
} = vi.hoisted(() => ({
  mockGetConfig: vi.fn(),
  mockTestConnection: vi.fn(),
  mockStartMigration: vi.fn(),
  mockGetTask: vi.fn(),
  mockToastSuccess: vi.fn(),
  mockToastError: vi.fn(),
  mockToastInfo: vi.fn(),
}))

// Mock only the API methods the section calls; the rest of the barrel stays
// absent because the component touches nothing else.
vi.mock('@/lib/api', () => ({
  api: {
    getRuntimeDatabaseConfig: mockGetConfig,
    testExternalDatabaseConnection: mockTestConnection,
    startDatabaseMigration: mockStartMigration,
    getTask: mockGetTask,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
    info: mockToastInfo,
  },
}))

// The section renders a FormNavigationGuard (useBlocker) for its dirty form;
// the test has no router, so stub the blocker in its idle state.
vi.mock('@tanstack/react-router', () => ({
  useBlocker: () => ({ status: 'idle', reset: vi.fn(), proceed: vi.fn() }),
}))

const sqliteRuntimeConfig = {
  active: { dialect: 'sqlite', connection: './data/metapi.db', ssl: false },
  saved: null,
  restartRequired: false,
}

const runningTask = {
  id: 'task-1',
  type: 'database_migration',
  title: '数据库迁移',
  status: 'running',
  message: 'Inserting 12 rows...',
  error: null,
  result: null,
  createdAt: '2026-08-20T10:00:00Z',
  updatedAt: '2026-08-20T10:00:01Z',
  logs: [
    {
      seq: 1,
      message: 'Direction: SQLite → PostgreSQL (forward migration)',
      createdAt: '2026-08-20T10:00:00Z',
    },
    {
      seq: 2,
      message: 'Reading source database...',
      createdAt: '2026-08-20T10:00:00Z',
    },
  ],
}

const succeededTask = {
  ...runningTask,
  status: 'succeeded',
  message: '数据库迁移 已完成',
  finishedAt: '2026-08-20T10:00:05Z',
  result: {
    dialect: 'postgres',
    connection: 'postgres://user:***@host:5432/db',
    overwrite: true,
    version: '0.16.2',
    timestamp: 1755684005,
    rows: { sites: 2, accounts: 1 },
  },
}

beforeAll(() => {
  // Radix/base-ui primitives query matchMedia on render; jsdom leaves it
  // undefined otherwise and the select crashes.
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
  mockGetConfig.mockReset()
  mockTestConnection.mockReset()
  mockStartMigration.mockReset()
  mockGetTask.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  mockToastInfo.mockReset()
  mockGetConfig.mockResolvedValue(sqliteRuntimeConfig)
  mockTestConnection.mockResolvedValue({ success: true })
  mockStartMigration.mockResolvedValue({
    success: true,
    message: 'ok',
    taskId: 'task-1',
  })
})

afterEach(() => cleanup())

function renderMigrationSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <DatabaseMigrationSection />
    </QueryClientProvider>
  )
}

async function fillConnection(connection: string) {
  const input = await screen.findByLabelText('Connection string')
  fireEvent.change(input, { target: { value: connection } })
}

// The confirm dialog action and the trigger share the 'Start migration'
// label; the dialog action is the last matching button in the tree.
function clickConfirmAction() {
  const confirmButtons = screen.getAllByRole('button', {
    name: 'Start migration',
  })
  const confirmButton = confirmButtons.at(-1)
  if (!confirmButton) {
    throw new Error('Confirm action button not found')
  }
  fireEvent.click(confirmButton)
}

describe('DatabaseMigrationSection — form and confirm flow', () => {
  it('renders with a postgres default target when the live runtime is sqlite', async () => {
    renderMigrationSection()

    expect(await screen.findByText('Data migration')).toBeInTheDocument()
    expect(
      screen.getByText('Migration source (live runtime database)')
    ).toBeInTheDocument()
    expect(screen.getByText('PostgreSQL')).toBeInTheDocument()
    // SSL checkbox only renders for postgres targets.
    expect(
      screen.getByRole('checkbox', { name: 'Enable SSL/TLS' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('checkbox', { name: 'Overwrite existing data' })
    ).toBeChecked()
  })

  it('rejects testing without a connection string', async () => {
    renderMigrationSection()

    fireEvent.click(
      await screen.findByRole('button', { name: 'Test connection' })
    )

    expect(
      await screen.findByText('Enter the connection string to test.')
    ).toBeInTheDocument()
    expect(mockTestConnection).not.toHaveBeenCalled()
  })

  it('tests the connection with the form values', async () => {
    renderMigrationSection()

    await fillConnection('postgres://user:pass@host:5432/db')
    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))

    await waitFor(() => {
      expect(mockTestConnection).toHaveBeenCalledWith({
        dialect: 'postgres',
        connectionString: 'postgres://user:pass@host:5432/db',
        ssl: false,
      })
    })
    await waitFor(() => {
      expect(mockToastSuccess).toHaveBeenCalledWith('Connection OK.')
    })
  })

  it('requires a confirm dialog before starting the migration', async () => {
    renderMigrationSection()

    await fillConnection('postgres://user:pass@host:5432/db')
    fireEvent.click(screen.getByRole('button', { name: 'Start migration' }))

    expect(
      await screen.findByText('Confirm data migration')
    ).toBeInTheDocument()
    expect(mockStartMigration).not.toHaveBeenCalled()

    // The destructive confirm action carries the same label as the trigger.
    clickConfirmAction()

    await waitFor(() => {
      expect(mockStartMigration).toHaveBeenCalledWith({
        dialect: 'postgres',
        connectionString: 'postgres://user:pass@host:5432/db',
        overwrite: true,
        ssl: false,
      })
    })
    await waitFor(() => {
      expect(mockToastInfo).toHaveBeenCalledWith(
        'Migration task started; it is running in the background.'
      )
    })
  })

  it('surfaces the localized same-target rejection instead of the raw server message', async () => {
    mockStartMigration.mockRejectedValueOnce({
      response: { data: { error: '目标数据库与当前运行库相同，无法迁移' } },
    })
    renderMigrationSection()

    await fillConnection('./data/metapi.db')
    fireEvent.click(screen.getByRole('button', { name: 'Start migration' }))
    await screen.findByText('Confirm data migration')
    clickConfirmAction()

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        'The target database is the same as the live runtime database; migration is not possible.'
      )
    })
  })
})

describe('DatabaseMigrationSection — background task polling', () => {
  it(
    'polls the task and renders the row summary on success',
    { timeout: 15_000 },
    async () => {
      mockGetTask
        .mockResolvedValueOnce({ success: true, task: runningTask })
        .mockResolvedValueOnce({ success: true, task: succeededTask })
      renderMigrationSection()

      await fillConnection('postgres://user:pass@host:5432/db')
      fireEvent.click(screen.getByRole('button', { name: 'Start migration' }))
      await screen.findByText('Confirm data migration')
      clickConfirmAction()

      await waitFor(() => {
        expect(mockGetTask).toHaveBeenCalledWith('task-1')
      })

      // Poll interval is 2s; wait for the terminal poll to land and surface.
      expect(
        await screen.findByText('Migration result', {}, { timeout: 6000 })
      ).toBeInTheDocument()
      expect(
        screen.getByText('Rows per table · 3 rows total')
      ).toBeInTheDocument()
      expect(screen.getByText('sites')).toBeInTheDocument()
      expect(screen.getByText('2')).toBeInTheDocument()

      await waitFor(() => {
        expect(mockToastSuccess).toHaveBeenCalledWith(
          'Data migration completed.'
        )
      })
    }
  )

  it('surfaces the task error on failure', { timeout: 15_000 }, async () => {
    mockGetTask
      .mockResolvedValueOnce({ success: true, task: runningTask })
      .mockResolvedValueOnce({
        success: true,
        task: {
          ...runningTask,
          status: 'failed',
          error: 'open target: connection refused',
          finishedAt: '2026-08-20T10:00:05Z',
        },
      })
    renderMigrationSection()

    await fillConnection('postgres://user:pass@host:5432/db')
    fireEvent.click(screen.getByRole('button', { name: 'Start migration' }))
    await screen.findByText('Confirm data migration')
    clickConfirmAction()

    await waitFor(
      () => {
        expect(mockToastError).toHaveBeenCalledWith(
          'open target: connection refused'
        )
      },
      { timeout: 6000 }
    )
  })
})

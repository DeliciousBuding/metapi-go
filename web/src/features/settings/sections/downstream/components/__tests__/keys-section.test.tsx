// Behavior test for the downstream API key Sheet's EDIT mode branching.
//
// KeysSection renders a single KeySheetForm sub-component that doubles as
// create and edit:
//   - editingKey === null  → create branch (full schema incl. secret `key`)
//   - editingKey !== null  → edit branch (editKeySchema omits `key`, calls
//                            api.updateDownstreamApiKey(id, partial))
//
// The edit branch was added in a separate commit; this test locks the
// pre-fill, the update call, and the secret-field suppression so the
// branching cannot silently regress to the create path. KeySheetForm is
// rendered inside a minimal <Sheet open><SheetContent> wrapper (the same
// context KeysSection provides in production) so the base-ui Dialog parts
// (SheetTitle/SheetDescription/SheetFooter) have their Root context, plus a
// fresh QueryClientProvider for useQueryClient/useMutation.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactElement } from 'react'
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
import { Sheet, SheetContent } from '@/components/ui/sheet'

import { KeyModelPolicyCell, KeySheetForm, KeyUsageCell } from '../keys-section'

// vi.hoisted keeps the mock fn identities stable across the factory's
// re-evaluation, so the vi.mock below can reference them by closure.
const { mockCreateKey, mockUpdateKey, mockToastSuccess, mockToastError } =
  vi.hoisted(() => ({
    mockCreateKey: vi.fn(),
    mockUpdateKey: vi.fn(),
    mockToastSuccess: vi.fn(),
    mockToastError: vi.fn(),
  }))

// Mock only the two API methods KeySheetForm actually calls. The rest of the
// `api` barrel stays absent because the form touches nothing else.
vi.mock('@/lib/api', () => ({
  api: {
    createDownstreamApiKey: mockCreateKey,
    updateDownstreamApiKey: mockUpdateKey,
  },
}))

// Avoid real sonner side effects (DOM portal + timers) under jsdom.
vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: mockToastError,
  },
}))

beforeAll(() => {
  // Radix/base-ui primitives query matchMedia on render; jsdom leaves it
  // undefined otherwise and the form layout crashes.
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
  mockCreateKey.mockReset()
  mockUpdateKey.mockReset()
  mockToastSuccess.mockReset()
  mockToastError.mockReset()
  // Both mutations resolve successfully so onSuccess → onDone fires.
  mockCreateKey.mockResolvedValue({})
  mockUpdateKey.mockResolvedValue({})
})

afterEach(() => cleanup())

// KeySheetForm needs a QueryClientProvider (useQueryClient + useMutation)
// AND a Sheet/Dialog.Root context — SheetTitle/SheetDescription/SheetFooter
// are base-ui Dialog parts that throw without <Sheet><SheetContent>. The
// react-dom createPortal mock in vitest.setup inlines SheetContent's portal
// so the form lands in the test DOM. A fresh QueryClient per render keeps
// cache state from leaking between cases.
function renderKeySheetForm(props: {
  editingKey: Parameters<typeof KeySheetForm>[0]['editingKey']
  onDone?: () => void
  onCreated?: (target: { id: number; name: string; keyMasked?: string }) => void
  onDirtyChange?: (dirty: boolean) => void
  candidateModels?: string[]
}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  const onDone = props.onDone ?? vi.fn()
  const onCreated = props.onCreated ?? vi.fn()
  return {
    onDone,
    onCreated,
    ...render(
      (
        <QueryClientProvider client={queryClient}>
          <Sheet open onOpenChange={() => {}}>
            <SheetContent>
              <KeySheetForm
                editingKey={props.editingKey}
                onDone={onDone}
                onCreated={onCreated}
                onDirtyChange={props.onDirtyChange}
                candidateModels={props.candidateModels}
              />
            </SheetContent>
          </Sheet>
        </QueryClientProvider>
      ) as ReactElement
    ),
  }
}

// A representative editing target: every optional field is populated so the
// pre-fill assertions can cover the inputs that actually render a value.
// NonNullable<...> narrows away the `null` create-mode value so `knownKey.id`
// is usable in the update-call assertion without a null guard.
const knownKey = {
  id: 42,
  name: 'My downstream key',
  keyMasked: 'sk-…abc',
  groupName: 'prod',
  enabled: true,
  maxRequests: 1000,
  maxCost: 50,
  usedRequests: 10,
  usedCost: 5,
  expiresAt: null,
  supportedModels: '["gpt-4o","gpt-*","re:^claude-"]',
} as NonNullable<Parameters<typeof KeySheetForm>[0]['editingKey']>

describe('KeySheetForm — edit mode', () => {
  it('pre-fills form fields from the editing key', () => {
    renderKeySheetForm({ editingKey: knownKey })

    expect(screen.getByLabelText('Name')).toHaveValue('My downstream key')
    expect(screen.getByLabelText('Group name')).toHaveValue('prod')
    // Numeric inputs report valueAsNumber; assert against numbers, not strings.
    expect(screen.getByLabelText('Max requests')).toHaveValue(1000)
    expect(screen.getByLabelText('Max cost')).toHaveValue(50)
    expect(screen.getByText('gpt-4o')).toBeInTheDocument()
    expect(screen.getByText('gpt-*')).toBeInTheDocument()
    expect(screen.getByText('re:^claude-')).toBeInTheDocument()
    expect(screen.getByTestId('model-policy-form-summary')).toHaveTextContent(
      '3 rules'
    )
  })

  it('hides the secret `key` field (and Generate button) in edit mode', () => {
    renderKeySheetForm({ editingKey: knownKey })

    // The `key` FormField is conditionally null in edit mode — the label,
    // input, and placeholder must all be absent.
    expect(screen.queryByLabelText('Key')).toBeNull()
    expect(screen.queryByPlaceholderText('sk-…')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Generate' })).toBeNull()
  })

  it('calls updateDownstreamApiKey (not create) with a key-less payload on submit', async () => {
    const { onDone } = renderKeySheetForm({ editingKey: knownKey })

    const form = document.querySelector('form')
    if (!form) {
      throw new Error('KeySheetForm did not render its <form>')
    }
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mockUpdateKey).toHaveBeenCalledTimes(1)
    })

    // First arg is the editing key's id; second is the partial payload that
    // must omit `key` and `description` (rotation/description are separate
    // surfaces and the backend treats absent fields as "preserve as-is").
    expect(mockUpdateKey).toHaveBeenCalledWith(
      knownKey.id,
      expect.objectContaining({
        name: 'My downstream key',
        groupName: 'prod',
        maxRequests: 1000,
        maxCost: 50,
        enabled: true,
        expiresAt: '',
        supportedModels: ['gpt-4o', 'gpt-*', 're:^claude-'],
      })
    )
    const updatePayload = mockUpdateKey.mock.calls[0][1] as Record<
      string,
      unknown
    >
    expect(updatePayload).not.toHaveProperty('key')
    expect(updatePayload).not.toHaveProperty('description')

    // The create path must not have been touched by the edit submit.
    expect(mockCreateKey).not.toHaveBeenCalled()

    // onSuccess invalidates + toasts + calls onDone; assert onDone so the
    // full success side-effect is locked (not just the network call).
    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    expect(mockToastError).not.toHaveBeenCalled()
  })
})

describe('KeySheetForm — create mode', () => {
  it('shows the key field and calls createDownstreamApiKey (not update) on submit', async () => {
    const { onDone } = renderKeySheetForm({ editingKey: null })

    // Create mode renders the secret `key` field + Generate button.
    expect(screen.getByLabelText('Key')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Generate' })).toBeInTheDocument()

    expect(screen.getByTestId('model-policy-form-summary')).toHaveTextContent(
      'No models authorized'
    )

    // Fill the two required fields (name non-empty, key ≥ 8 chars) so the
    // zodResolver passes and the submit reaches the mutation.
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-12345678' },
    })

    const form = document.querySelector('form')
    if (!form) {
      throw new Error('KeySheetForm did not render its <form>')
    }
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mockCreateKey).toHaveBeenCalledTimes(1)
    })

    // Create payload carries the full form values, INCLUDING the `key`
    // field — the mirror of the edit-mode omission assertion.
    expect(mockCreateKey).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'New key',
        key: 'sk-12345678',
        supportedModels: [],
      })
    )
    const createPayload = mockCreateKey.mock.calls[0][0] as Record<
      string,
      unknown
    >
    expect(createPayload).toHaveProperty('key')

    expect(mockUpdateKey).not.toHaveBeenCalled()
    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
  })

  it('persists exact, glob, and regex rules selected from inventory or entered manually', async () => {
    renderKeySheetForm({
      editingKey: null,
      candidateModels: ['gpt-4o', 'gpt-4o-mini', 'claude-sonnet-4'],
    })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Pattern key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-pattern-key' },
    })

    const modelRuleInput = screen.getByLabelText('Model access')
    fireEvent.change(modelRuleInput, { target: { value: 'gpt-4o' } })
    fireEvent.click(screen.getByRole('option', { name: '+ gpt-4o' }))

    fireEvent.change(modelRuleInput, { target: { value: 'claude-*' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    fireEvent.change(modelRuleInput, { target: { value: 're:^deepseek-' } })
    fireEvent.keyDown(modelRuleInput, { key: 'Enter', code: 'Enter' })

    expect(screen.getByText('gpt-4o')).toBeInTheDocument()
    expect(screen.getByText('claude-*')).toBeInTheDocument()
    expect(screen.getByText('re:^deepseek-')).toBeInTheDocument()
    expect(screen.getByTestId('model-policy-form-summary')).toHaveTextContent(
      '3 rules'
    )

    const form = document.querySelector('form')
    if (!form) throw new Error('KeySheetForm did not render its <form>')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mockCreateKey).toHaveBeenCalledTimes(1)
    })
    expect(mockCreateKey).toHaveBeenCalledWith(
      expect.objectContaining({
        supportedModels: ['gpt-4o', 'claude-*', 're:^deepseek-'],
      })
    )
  })

  it('requires an explicit wildcard rule to authorize all models', async () => {
    renderKeySheetForm({ editingKey: null })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'All models key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-all-models' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Allow all' }))

    expect(screen.getByTestId('model-policy-form-summary')).toHaveTextContent(
      'All models'
    )
    expect(screen.getByText('*')).toBeInTheDocument()

    const form = document.querySelector('form')
    if (!form) throw new Error('KeySheetForm did not render its <form>')
    fireEvent.submit(form)

    await waitFor(() => {
      expect(mockCreateKey).toHaveBeenCalledTimes(1)
    })
    expect(mockCreateKey).toHaveBeenCalledWith(
      expect.objectContaining({ supportedModels: ['*'] })
    )
  })

  it('reports the created key to onCreated so KeysSection can auto-open Connect', async () => {
    // The real backend envelope: POST /api/downstream-keys re-reads the
    // inserted row and answers { success, item }.
    mockCreateKey.mockResolvedValue({
      success: true,
      item: { id: 7, name: 'New key', keyMasked: 'sk-…5678' },
    })

    const { onDone, onCreated } = renderKeySheetForm({ editingKey: null })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-12345678' },
    })

    const form = document.querySelector('form')
    if (!form) {
      throw new Error('KeySheetForm did not render its <form>')
    }
    fireEvent.submit(form)

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledWith({
        id: 7,
        name: 'New key',
        keyMasked: 'sk-…5678',
      })
    })
    // The sheet still closes exactly once — Connect opens as the sheet
    // dismisses, not instead of it.
    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
  })

  it('skips onCreated (keeps toast-only feedback) when the response lacks the created row', async () => {
    // Legacy/empty envelope: nothing to build a dialog target from, and
    // nothing may be fabricated.
    mockCreateKey.mockResolvedValue({ success: true })

    const { onDone, onCreated } = renderKeySheetForm({ editingKey: null })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-12345678' },
    })

    const form = document.querySelector('form')
    if (!form) {
      throw new Error('KeySheetForm did not render its <form>')
    }
    fireEvent.submit(form)

    await waitFor(() => {
      expect(onDone).toHaveBeenCalledTimes(1)
    })
    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    expect(onCreated).not.toHaveBeenCalled()
  })
})

describe('KeySheetForm — dirty reporting', () => {
  it('reports dirty state changes through onDirtyChange', () => {
    const onDirtyChange = vi.fn()
    renderKeySheetForm({ editingKey: null, onDirtyChange })

    // Initial mount reports a clean form.
    expect(onDirtyChange).toHaveBeenLastCalledWith(false)

    // Editing a field flips the report to dirty.
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'My new key' },
    })
    expect(onDirtyChange).toHaveBeenLastCalledWith(true)
  })
})

// KeyUsageCell locks the 24h usage line rendering: when the backend returns
// usage24h the requests/tokens/cost surface verbatim; when the field is
// absent (older server or failed aggregate) the line degrades to zeros
// instead of rendering "undefined".
describe('KeyUsageCell — 24h usage line', () => {
  const baseItem = {
    id: 1,
    name: 'usage key',
    enabled: true,
    usedRequests: 10,
    maxRequests: 100,
    usedCost: 2,
    maxCost: 50,
  } as const

  it('renders the 24h request/token/cost summary when usage24h is present', () => {
    render(
      <KeyUsageCell
        item={{
          ...baseItem,
          usage24h: { requests: 7, tokens: 1234, cost: 0.42 },
        }}
      />
    )

    expect(
      screen.getByText('24h: 7 req · 1234 tok · $0.42')
    ).toBeInTheDocument()
  })

  it('falls back to zeros when usage24h is absent', () => {
    render(<KeyUsageCell item={baseItem} />)

    expect(screen.getByText('24h: 0 req · 0 tok · $0')).toBeInTheDocument()
  })
})

describe('KeyModelPolicyCell — fail-closed summaries', () => {
  it('does not describe an empty policy as all models', () => {
    render(<KeyModelPolicyCell supportedModels={[]} />)

    expect(screen.getByText('No models authorized')).toBeInTheDocument()
    expect(screen.queryByText('All models')).toBeNull()
  })

  it('distinguishes an explicit wildcard from a finite rule set', () => {
    render(
      <div>
        <KeyModelPolicyCell supportedModels={['*']} />
        <KeyModelPolicyCell supportedModels={['gpt-4o', 'claude-*']} />
        <KeyModelPolicyCell supportedModels={[]} allowedRouteIds={[12, 13]} />
      </div>
    )

    expect(screen.getByText('All models')).toBeInTheDocument()
    expect(screen.getByText('2 rules')).toBeInTheDocument()
    expect(screen.getByText('2 route grants')).toBeInTheDocument()
  })
})

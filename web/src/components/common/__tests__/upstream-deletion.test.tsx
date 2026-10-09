import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import type { UpstreamDeletionPreview } from '@/lib/api/upstream-lifecycle'

import { useUpstreamDeletion } from '../upstream-deletion'

const mocks = vi.hoisted(() => ({
  preview: vi.fn(),
  remove: vi.fn(),
  leaf: vi.fn(),
  deleted: vi.fn(),
  success: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  api: {
    previewUpstreamDeletion: mocks.preview,
    deleteUpstreamEntity: mocks.remove,
  },
}))
vi.mock('@/lib/toast', () => ({ toast: { success: mocks.success } }))

const original: UpstreamDeletionPreview = {
  kind: 'route',
  id: 7,
  counts: { routes: 1, groups: 1, members: 2, downstreamKeys: 3 },
  affectedRouteIds: [7],
  revision: 'first-revision',
  requiresCascade: true,
}
function Harness() {
  const deletion = useUpstreamDeletion()
  return (
    <>
      <button
        type='button'
        disabled={deletion.isPending}
        onClick={() => {
          void deletion.requestDeletion(
            { kind: 'route', id: 7, name: 'Imported route' },
            mocks.leaf,
            mocks.deleted
          )
        }}
      >
        Remove route
      </button>
      {deletion.dialog}
    </>
  )
}
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  render(
    <QueryClientProvider client={client}>
      <Harness />
    </QueryClientProvider>
  )
  return invalidate
}
async function settle() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0)
  })
}
async function open() {
  fireEvent.click(screen.getByRole('button', { name: 'Remove route' }))
  await settle()
}
async function confirmWord() {
  fireEvent.change(screen.getByPlaceholderText('DELETE'), {
    target: { value: 'DELETE' },
  })
  for (let i = 0; i < 3; i++) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
  }
}
function conflict(preview: UpstreamDeletionPreview | undefined) {
  return {
    isAxiosError: true,
    response: { status: 409, data: { error: 'changed', preview } },
  }
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
  vi.useFakeTimers()
  vi.clearAllMocks()
  mocks.preview.mockReset().mockResolvedValue(original)
  mocks.remove.mockReset().mockResolvedValue({ ...original, success: true })
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('upstream deletion review', () => {
  it('does not delete when preview fails and retries a leaf through the existing caller', async () => {
    mocks.preview
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce({ ...original, requiresCascade: false })
    mount()
    await open()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load the deletion impact'
    )
    expect(mocks.leaf).not.toHaveBeenCalled()
    expect(mocks.remove).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await settle()
    expect(mocks.leaf).toHaveBeenCalledWith(
      expect.objectContaining({ requiresCascade: false }),
      expect.any(Function)
    )
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.remove).not.toHaveBeenCalled()
  })

  it('disables requests and dismissal while the preview is pending', async () => {
    mocks.preview.mockReturnValue(new Promise(() => {}))
    mount()
    await open()
    expect(screen.getByText('Remove route')).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByText('Checking related records…')).toBeInTheDocument()
    expect(mocks.remove).not.toHaveBeenCalled()
  })

  it('requires the countdown and exact word, sends the reviewed revision, and blocks duplicate submission', async () => {
    let finish!: (value: unknown) => void
    mocks.remove.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve
      })
    )
    const invalidate = mount()
    await open()
    expect(screen.getByText('Groups deleted')).toBeInTheDocument()
    expect(screen.getByText('Affected routes: #7')).toBeInTheDocument()
    fireEvent.change(screen.getByPlaceholderText('DELETE'), {
      target: { value: 'DELETE' },
    })
    expect(screen.getByRole('button', { name: 'Confirm in 3s' })).toBeDisabled()
    await confirmWord()
    fireEvent.change(screen.getByPlaceholderText('DELETE'), {
      target: { value: 'delete' },
    })
    expect(
      screen.getByRole('button', { name: 'Delete these records' })
    ).toBeDisabled()
    fireEvent.change(screen.getByPlaceholderText('DELETE'), {
      target: { value: 'DELETE' },
    })
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await settle()
    expect(mocks.remove).toHaveBeenCalledExactlyOnceWith('route', 7, {
      cascade: true,
      expectedRevision: 'first-revision',
    })
    expect(screen.getByRole('button', { name: 'Deleting…' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
    expect(screen.getByPlaceholderText('DELETE')).toBeDisabled()
    expect(
      screen.queryByRole('button', { name: 'Close' })
    ).not.toBeInTheDocument()
    await act(async () => {
      finish({ success: true })
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(mocks.deleted).toHaveBeenCalledOnce()
    for (const key of ['imported-upstreams', 'routes', 'channels']) {
      expect(invalidate).toHaveBeenCalledWith({ queryKey: [key] })
    }
  })

  it('shows a 409 preview and requires a new word and countdown even when the new plan is a leaf', async () => {
    const updated = {
      ...original,
      revision: 'new-revision',
      requiresCascade: false,
      counts: { routes: 1, credentials: 4 },
    }
    mocks.remove
      .mockRejectedValueOnce(conflict(updated))
      .mockResolvedValueOnce({ success: true })
    mount()
    await open()
    await confirmWord()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await settle()
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Related records changed'
    )
    expect(screen.getByText('Credentials deleted')).toBeInTheDocument()
    expect(screen.queryByText('Groups deleted')).not.toBeInTheDocument()
    expect(screen.getByPlaceholderText('DELETE')).toHaveValue('')
    expect(screen.getByRole('button', { name: 'Confirm in 3s' })).toBeDisabled()
    expect(mocks.remove).toHaveBeenCalledOnce()
    expect(mocks.leaf).not.toHaveBeenCalled()
    await confirmWord()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await settle()
    expect(mocks.remove).toHaveBeenLastCalledWith('route', 7, {
      cascade: false,
      expectedRevision: 'new-revision',
    })
    expect(mocks.remove).toHaveBeenCalledTimes(2)
  })

  it('retains the window on deletion failure without retrying automatically', async () => {
    mocks.remove.mockRejectedValue(new Error('unavailable'))
    mount()
    await open()
    await confirmWord()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await settle()
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent('Deletion failed')
    expect(mocks.remove).toHaveBeenCalledOnce()
    expect(mocks.deleted).not.toHaveBeenCalled()
  })

  it('reopens review when the deferred leaf commit discovers new dependencies', async () => {
    mocks.preview.mockResolvedValue({ ...original, requiresCascade: false })
    mocks.remove.mockRejectedValue(
      conflict({ ...original, revision: 'after-undo-window' })
    )
    mount()
    await open()
    const commit = mocks.leaf.mock.calls[0][1] as () => Promise<unknown>
    expect(mocks.remove).not.toHaveBeenCalled()
    await act(async () => {
      await expect(commit()).rejects.toMatchObject({
        response: { status: 409 },
      })
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Related records changed'
    )
    expect(screen.getByPlaceholderText('DELETE')).toHaveValue('')
    expect(mocks.remove).toHaveBeenCalledOnce()
  })

  it('requires a fresh preview when a conflict supplies no usable revision', async () => {
    mocks.remove.mockRejectedValue(conflict(undefined))
    mount()
    await open()
    await confirmWord()
    fireEvent.click(
      screen.getByRole('button', { name: 'Delete these records' })
    )
    await settle()
    expect(screen.queryByPlaceholderText('DELETE')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
    expect(mocks.remove).toHaveBeenCalledOnce()
  })
})

// GCP service-account credential form (route-decision F batch): the kind
// option swaps the secret input for an SA JSON textarea, submit carries
// kind+material, and empty material stays client-side.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { UpstreamCredentials } from '../components/upstream-credentials'

const state = vi.hoisted(() => ({
  credentials: vi.fn(),
  createCredential: vi.fn(),
  updateCredential: vi.fn(),
  preview: vi.fn(),
  remove: vi.fn(),
  detail: vi.fn(),
  dirty: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getImportedCredentials: state.credentials,
    createUpstreamCredential: state.createCredential,
    updateImportedCredential: state.updateCredential,
    previewUpstreamDeletion: state.preview,
    deleteUpstreamEntity: state.remove,
    getImportedUpstream: state.detail,
  },
}))
vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}))

let client: QueryClient
function mountPanel() {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 } },
  })
  return render(
    <QueryClientProvider client={client}>
      <UpstreamCredentials id={4} active onDirtyChange={state.dirty} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  Element.prototype.scrollIntoView = vi.fn()
  state.credentials.mockResolvedValue({ items: [] })
  state.detail.mockResolvedValue({ id: 4, name: 'vertex-fixture' })
  state.createCredential.mockResolvedValue({ id: 9 })
})

afterEach(() => {
  cleanup()
  client?.clear()
})

function submitCredentialForm() {
  const submit = screen
    .getAllByRole('button', { name: 'Add credential' })
    .find((button) => button.getAttribute('type') === 'submit')
  if (!submit) throw new Error('submit button missing')
  fireEvent.click(submit)
}

async function chooseGCPKind() {
  fireEvent.click(screen.getByRole('combobox'))
  const option = await screen.findByRole('option', {
    name: 'GCP service account',
  })
  fireEvent.pointerDown(option)
  fireEvent.click(option)
}

describe('GCP service-account credential form', () => {
  it('offers the GCP kind and swaps the secret input for a JSON textarea', async () => {
    mountPanel()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Add credential' })
    )
    await chooseGCPKind()

    expect(
      screen.getByPlaceholderText('{"type":"service_account",...}')
    ).toBeInTheDocument()
    expect(screen.queryByLabelText('API Key')).not.toBeInTheDocument()
  })

  it('submits kind and material, and never the exchanged token', async () => {
    mountPanel()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Add credential' })
    )
    fireEvent.change(screen.getByLabelText(/name/i), {
      target: { value: 'prod-sa' },
    })
    await chooseGCPKind()
    const material =
      '{"type":"service_account","client_email":"a@b.iam","private_key":"pem","token_uri":"https://oauth2.googleapis.com/token"}'
    fireEvent.change(
      screen.getByPlaceholderText('{"type":"service_account",...}'),
      { target: { value: material } }
    )
    submitCredentialForm()

    await waitFor(() => expect(state.createCredential).toHaveBeenCalledTimes(1))
    const [channelId, payload] = state.createCredential.mock.calls[0]
    expect(channelId).toBe(4)
    expect(payload.kind).toBe('gcp_service_account')
    expect(payload.gcpServiceAccount).toBe(material)
    expect(payload.apiKey).toBeUndefined()
    expect(payload.oauth).toBeUndefined()
  })

  it('blocks submit when the material is empty', async () => {
    mountPanel()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Add credential' })
    )
    fireEvent.change(screen.getByLabelText(/name/i), {
      target: { value: 'prod-sa' },
    })
    await chooseGCPKind()
    submitCredentialForm()

    await waitFor(() => expect(state.createCredential).not.toHaveBeenCalled())
  })
})

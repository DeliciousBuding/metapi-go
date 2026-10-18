// Behavior test for the shared master-token re-confirmation host (#1034).
//
// Features request a reauth prompt, the provider renders one shared dialog and
// hands the entered token to the replay callback. The token is passed straight
// through — never stored in context or persisted — and a rejected replay keeps
// the dialog open with its inline error.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import {
  ReauthProvider,
  useReauth,
  type ReauthRequest,
} from '../reauth-provider'

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

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

function Harness({ request }: { request: ReauthRequest }) {
  const { requestReauth } = useReauth()
  return (
    <button type='button' onClick={() => requestReauth(request)}>
      Request confirmation
    </button>
  )
}

function renderHarness(request: ReauthRequest) {
  return render(
    <ReauthProvider>
      <Harness request={request} />
    </ReauthProvider>
  )
}

async function openAndSubmit(token: string) {
  fireEvent.click(screen.getByRole('button', { name: 'Request confirmation' }))
  const input = await screen.findByLabelText(text('reauth.tokenLabel'))
  fireEvent.change(input, { target: { value: token } })
  fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))
}

describe('ReauthProvider', () => {
  it('replays the pending operation with the entered token and closes', async () => {
    const replay = vi.fn().mockResolvedValue(undefined)
    renderHarness({ description: 'Sensitive operation', replay })

    await openAndSubmit('  master-token  ')

    await waitFor(() => expect(replay).toHaveBeenCalledWith('master-token'))
    await waitFor(() =>
      expect(
        screen.queryByLabelText(text('reauth.tokenLabel'))
      ).not.toBeInTheDocument()
    )
  })

  it('keeps the dialog open with an inline error when the token is rejected', async () => {
    const replay = vi.fn().mockRejectedValue(reauthError())
    renderHarness({ description: 'Sensitive operation', replay })

    await openAndSubmit('wrong-token')

    expect(await screen.findByText(text('reauth.errorInvalid'))).toBeVisible()
    expect(screen.getByLabelText(text('reauth.tokenLabel'))).toBeVisible()
  })

  it('does not replay when the operator cancels', async () => {
    const replay = vi.fn()
    renderHarness({ description: 'Sensitive operation', replay })

    fireEvent.click(
      screen.getByRole('button', { name: 'Request confirmation' })
    )
    await screen.findByLabelText(text('reauth.tokenLabel'))
    fireEvent.click(screen.getByRole('button', { name: text('reauth.cancel') }))

    await waitFor(() =>
      expect(
        screen.queryByLabelText(text('reauth.tokenLabel'))
      ).not.toBeInTheDocument()
    )
    expect(replay).not.toHaveBeenCalled()
  })
})

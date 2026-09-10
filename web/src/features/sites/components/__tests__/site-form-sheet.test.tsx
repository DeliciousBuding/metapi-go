// Regression tests for the add/edit site form dialog. Guards the gap-1
// round-trip of `customHeadersOverrideRequestHeaders`, Zod submission
// errors, the create payload reaching `onCreated`, and the dirty-close
// confirm guard. Mocks only the sites api + toast; keeps the real
// RHF + Zod + dirty-close-hook code paths under test.
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

import '@/i18n/config'

import type { Site } from '../../types'
import { SiteFormSheet } from '../site-form-sheet'

const { mockCreateMutate, mockUpdateMutate, mockDetectMutate, mockToastError } =
  vi.hoisted(() => ({
    mockCreateMutate: vi.fn(),
    mockUpdateMutate: vi.fn(),
    mockDetectMutate: vi.fn(),
    mockToastError: vi.fn(),
  }))

vi.mock('../../api', () => ({
  useCreateSite: () => ({ mutateAsync: mockCreateMutate, isPending: false }),
  useUpdateSite: () => ({ mutateAsync: mockUpdateMutate, isPending: false }),
  useDetectSite: () => ({ mutateAsync: mockDetectMutate, isPending: false }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: mockToastError,
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

beforeAll(() => {
  // base-ui Dialog / AlertDialog / Select need matchMedia under jsdom.
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
  mockCreateMutate.mockReset()
  mockUpdateMutate.mockReset()
  mockDetectMutate.mockReset()
  mockToastError.mockReset()
  // Default: detection returns nothing so the platform stays manual.
  mockDetectMutate.mockResolvedValue({})
})

afterEach(() => cleanup())

function typeField(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

describe('SiteFormSheet Zod submission errors', () => {
  it('renders the nameRequired error when submitting with an empty name', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(screen.getByText('Please enter a name.')).toBeInTheDocument()
    })
    // Validation failure must short-circuit before the create mutation fires.
    expect(mockCreateMutate).not.toHaveBeenCalled()
    expect(mockToastError).toHaveBeenCalledTimes(1)
  })
})

describe('SiteFormSheet create payload', () => {
  it('calls onCreated with the created site after a valid submit', async () => {
    const createdSite: Site = {
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: '',
      status: 'active',
    }
    mockCreateMutate.mockResolvedValue(createdSite)
    const onCreated = vi.fn()

    render(
      <SiteFormSheet
        open
        onOpenChange={vi.fn()}
        editingSite={null}
        onCreated={onCreated}
      />
    )

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'My Site')
    typeField('URL', 'https://example.com')

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    // The payload must carry the entered fields plus the default
    // `customHeadersOverrideRequestHeaders: false` (gap-1 round-trip
    // contract: create sends the schema's boolean, never undefined).
    const payload = mockCreateMutate.mock.calls[0]?.[0]
    expect(payload).toMatchObject({
      name: 'My Site',
      url: 'https://example.com',
      customHeadersOverrideRequestHeaders: false,
    })
    expect(onCreated).toHaveBeenCalledWith(createdSite)
  })
})

describe('SiteFormSheet platform picker', () => {
  // Canonical adapter platforms from platform/registry.go `orderedPlatformNames`.
  const CANONICAL_PLATFORMS = [
    'openai',
    'codex',
    'claude',
    'gemini',
    'gemini-cli',
    'antigravity',
    'grok',
    'cliproxyapi',
    'sensetime',
    'anyrouter',
    'done-hub',
    'one-hub',
    'veloera',
    'new-api',
    'sub2api',
    'one-api',
  ]

  it('lists the 16 canonical platforms in the platform select', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    const platformSelect = await screen.findByRole('combobox', {
      name: 'Platform',
    })

    fireEvent.mouseDown(platformSelect)

    for (const platform of CANONICAL_PLATFORMS) {
      expect(
        await screen.findByRole('option', { name: platform })
      ).toBeInTheDocument()
    }
  })

  it('sets the platform form value when a canonical platform is selected', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: 'claude',
      status: 'active',
    })

    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'My Site')
    typeField('URL', 'https://example.com')

    fireEvent.mouseDown(
      await screen.findByRole('combobox', { name: 'Platform' })
    )
    const claudeOption = await screen.findByRole('option', { name: 'claude' })
    fireEvent.pointerDown(claudeOption)
    fireEvent.click(claudeOption)

    await waitFor(() => {
      expect(
        screen.getByRole('combobox', { name: 'Platform' })
      ).toHaveTextContent('claude')
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    expect(mockCreateMutate.mock.calls[0]?.[0]).toMatchObject({
      platform: 'claude',
    })
  })

  it('keeps manual entry for unknown platforms via the custom toggle', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 42,
      name: 'My Site',
      url: 'https://example.com',
      platform: 'my-unknown-platform',
      status: 'active',
    })

    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'My Site')
    typeField('URL', 'https://example.com')

    fireEvent.click(screen.getByRole('button', { name: 'Enter manually' }))

    const platformInput = screen.getByLabelText('Platform')
    fireEvent.change(platformInput, {
      target: { value: 'my-unknown-platform' },
    })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    expect(mockCreateMutate.mock.calls[0]?.[0]).toMatchObject({
      platform: 'my-unknown-platform',
    })
  })
})

describe('SiteFormSheet post-refresh probe latency threshold (gap-11)', () => {
  it('renders the latency threshold field when probe is enabled and round-trips the value into the payload', async () => {
    // A successful create resolves with a minimal site object; only the
    // payload contract is asserted, not the created echo.
    mockCreateMutate.mockResolvedValue({
      id: 99,
      name: 'Probe site',
      url: 'https://probe.example',
      platform: '',
      status: 'active',
    })

    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    // Fill the required fields so a valid submit can fire.
    typeField('Name', 'Probe site')
    typeField('URL', 'https://probe.example')

    // The latency threshold field must NOT render while the probe is off.
    expect(
      screen.queryByLabelText('Probe latency threshold (ms)')
    ).not.toBeInTheDocument()

    // Toggle the post-refresh probe switch on; the conditional block
    // (model + scope + latency threshold) must now render.
    fireEvent.click(screen.getByRole('switch', { name: 'Post-refresh probe' }))

    const latencyField = await screen.findByLabelText(
      'Probe latency threshold (ms)'
    )
    expect(latencyField).toBeInTheDocument()

    // Enter a threshold; the numeric onChange (valueAsNumber) must
    // round-trip the entered value into the submit payload.
    fireEvent.change(latencyField, { target: { value: '1500' } })

    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => {
      expect(mockCreateMutate).toHaveBeenCalledTimes(1)
    })

    const payload = mockCreateMutate.mock.calls[0]?.[0]
    expect(payload).toMatchObject({
      name: 'Probe site',
      url: 'https://probe.example',
      postRefreshProbeEnabled: true,
      postRefreshProbeLatencyThresholdMs: 1500,
    })
  })
})

describe('SiteFormSheet edit round-trip (gap-1)', () => {
  it('checks the override-headers switch when editing a site with it enabled', async () => {
    const editingSite: Site = {
      id: 7,
      name: 'Existing site',
      url: 'https://existing.example',
      platform: 'openai',
      status: 'active',
      customHeaders: '{"X-Test":"1"}',
      customHeadersOverrideRequestHeaders: true,
    }

    render(
      <SiteFormSheet open onOpenChange={vi.fn()} editingSite={editingSite} />
    )

    const overrideSwitch = await screen.findByRole('switch', {
      name: 'Override request headers',
    })

    await waitFor(() => {
      expect(overrideSwitch).toHaveAttribute('aria-checked', 'true')
    })
  })
})

describe('SiteFormSheet dirty-close guard', () => {
  it('opens the discard confirm when closing with unsaved edits', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'dirty value')

    // Trigger close via the dialog's close (X) button. The dirty-close
    // guard must intercept and show the confirm instead of discarding.
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))

    await waitFor(() => {
      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument()
    })
  })

  it('opens the discard confirm when Cancel is clicked with unsaved edits', async () => {
    const onOpenChange = vi.fn()
    render(
      <SiteFormSheet open onOpenChange={onOpenChange} editingSite={null} />
    )

    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toBeInTheDocument()
    })

    typeField('Name', 'dirty value')

    // The explicit Cancel button must route through the same dirty-close
    // guard as Esc/X — never silently discard the input (issue #889).
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.getByText('Discard unsaved changes?')).toBeInTheDocument()
    })
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})

describe('SiteFormSheet custom headers examples (#1132)', () => {
  // Read-only snippets compiled into the bundle — no template entity, no
  // table, no setting. Each entry's accessible name comes from the
  // `sites.form.customHeadersExampleInsert` aria-label, not the visible
  // product name, so a screen-reader user hears what the click will do.
  const EXAMPLE_CLIENTS = ['Claude Code', 'Codex CLI', 'Gemini CLI'] as const

  function exampleButton(client: string) {
    return screen.getByRole('button', { name: `Insert ${client} example` })
  }

  async function renderEmptySheet() {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    const headers = await screen.findByLabelText('Custom headers')
    await waitFor(() => expect(headers).toBeInTheDocument())
    return headers
  }

  it('offers one enabled insert entry per built-in CLI example', async () => {
    const headers = await renderEmptySheet()
    expect(headers).toHaveValue('')

    for (const client of EXAMPLE_CLIENTS) {
      expect(exampleButton(client)).toBeEnabled()
    }
  })

  it('fills an empty field with the chosen example without touching the backend', async () => {
    const headers = await renderEmptySheet()

    fireEvent.click(exampleButton('Claude Code'))

    await waitFor(() =>
      expect(headers).toHaveValue(
        '{"User-Agent":"claude-cli/<VERSION>","x-app":"cli"}'
      )
    )
    // Inserting an example is a pure textarea fill: it must not create, update
    // or sync anything, because the snippets are not persisted entities.
    expect(mockCreateMutate).not.toHaveBeenCalled()
    expect(mockUpdateMutate).not.toHaveBeenCalled()
  })

  it('treats whitespace-only content as empty', async () => {
    const headers = await renderEmptySheet()
    fireEvent.change(headers, { target: { value: '  \n ' } })

    await waitFor(() => expect(exampleButton('Codex CLI')).toBeEnabled())

    fireEvent.click(exampleButton('Codex CLI'))

    await waitFor(() =>
      expect(headers).toHaveValue(
        '{"User-Agent":"codex-cli/<VERSION>","originator":"codex_cli_rs"}'
      )
    )
  })

  it('refuses to touch a field the user already filled (fill-only: never merge, never replace)', async () => {
    const headers = await renderEmptySheet()
    const written = '{"User-Agent":"my-handwritten-client","X-Keep":"mine"}'
    fireEvent.change(headers, { target: { value: written } })

    await waitFor(() => {
      for (const client of EXAMPLE_CLIENTS) {
        expect(exampleButton(client)).toBeDisabled()
      }
    })

    // Every entry is disabled, and a click on a disabled control must still
    // leave the hand-written JSON byte-identical.
    for (const client of EXAMPLE_CLIENTS) {
      fireEvent.click(exampleButton(client))
    }
    expect(headers).toHaveValue(written)
  })

  it("keeps an existing site's stored headers locked and intact in edit mode", async () => {
    const stored = '{"User-Agent":"claude-cli/1.2.3"}'
    const editingSite: Site = {
      id: 11,
      name: 'Existing site',
      url: 'https://existing.example',
      platform: 'claude',
      status: 'active',
      customHeaders: stored,
    }

    render(
      <SiteFormSheet open onOpenChange={vi.fn()} editingSite={editingSite} />
    )

    const headers = await screen.findByLabelText('Custom headers')
    await waitFor(() => expect(headers).toHaveValue(stored))

    for (const client of EXAMPLE_CLIENTS) {
      expect(exampleButton(client)).toBeDisabled()
      fireEvent.click(exampleButton(client))
    }
    expect(headers).toHaveValue(stored)
  })

  it('inserts valid JSON objects that are obviously placeholders and carry only injectable headers', async () => {
    // Mirrors platform.IsDeniedCustomHeader (data plane) and
    // service.isReservedPlatformCustomHeader (assembly side). An example that
    // suggested one of these would be dropped silently on the way upstream, so
    // it must never ship. Keep in step with those two Go predicates.
    const DENIED_HEADERS = new Set([
      'authorization',
      'host',
      'content-length',
      'content-type',
      'transfer-encoding',
      'accept-encoding',
      'connection',
      'cookie',
      'keep-alive',
      'te',
      'trailer',
      'trailers',
      'upgrade',
      'new-api-user',
    ])

    for (const client of EXAMPLE_CLIENTS) {
      const headers = await renderEmptySheet()
      fireEvent.click(exampleButton(client))
      await waitFor(() => expect(headers).not.toHaveValue(''))
      const inserted = (headers as HTMLTextAreaElement).value

      // Must satisfy the field's own `isEmptyOrValidJson` schema rule, i.e. a
      // JSON object — never an array, a bare string or invalid JSON. Inserting
      // an example must not be able to put the form into an error state.
      const parsed: unknown = JSON.parse(inserted)
      expect(parsed, `${client}: must parse`).not.toBeNull()
      expect(parsed, `${client}: must be an object`).toBeTypeOf('object')
      expect(Array.isArray(parsed), `${client}: must not be an array`).toBe(
        false
      )

      const names = Object.keys(parsed as Record<string, unknown>)
      expect(
        names.length,
        `${client}: must carry at least one header`
      ).toBeGreaterThan(0)
      for (const name of names) {
        const lower = name.toLowerCase()
        expect(
          DENIED_HEADERS.has(lower),
          `${client}: ${name} is not site-injectable`
        ).toBe(false)
        expect(
          lower.startsWith('proxy-'),
          `${client}: ${name} is a Proxy-* header`
        ).toBe(false)
      }

      // #1132's core constraint: an example must read as a placeholder, never
      // as an authoritative current value. A baked-in version rots with the
      // next upstream CLI release, and a stale template is worse than none
      // because users treat it as the real value.
      expect(inserted, `${client}: must contain a <PLACEHOLDER> token`).toMatch(
        /<[A-Z][A-Z_]*>/
      )
      expect(
        inserted,
        `${client}: must not bake in a concrete version`
      ).not.toMatch(/\d+\.\d+/)

      cleanup()
    }
  })
})

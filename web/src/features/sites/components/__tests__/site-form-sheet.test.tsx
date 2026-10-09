// Regression tests for the add/edit site form dialog. Guards the gap-1
// round-trip of `customHeadersOverrideRequestHeaders`, Zod submission
// errors, the create payload reaching `onCreated`, and the dirty-close
// confirm guard. Mocks only the sites api + toast; keeps the real
// RHF + Zod + dirty-close-hook code paths under test.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import i18n from 'i18next'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'
import type { SiteInitializationPreset } from '@/lib/api/sites'

import type { Site } from '../../types'
import { SiteFormSheet } from '../site-form-sheet'

const initializationPresets: SiteInitializationPreset[] = [
  {
    id: 'codingplan-openai',
    label: 'Aliyun CodingPlan / OpenAI',
    providerLabel: 'Aliyun CodingPlan',
    platform: 'openai',
    defaultUrl: 'https://coding.dashscope.aliyuncs.com/v1',
    recommendedSkipModelFetch: true,
    recommendedModels: [],
    docsUrl: '',
  },
  {
    id: 'xiaomi-token-plan-claude',
    label: 'Xiaomi Token Plan / Claude',
    providerLabel: 'Xiaomi Token Plan',
    platform: 'claude',
    defaultUrl: 'https://tokenplan.example.com/anthropic',
    recommendedSkipModelFetch: true,
    recommendedModels: [],
    docsUrl: '',
  },
  {
    id: 'deepseek-openai',
    label: 'DeepSeek / OpenAI',
    providerLabel: 'DeepSeek',
    platform: 'openai',
    defaultUrl: 'https://api.deepseek.com/v1',
    recommendedSkipModelFetch: true,
    recommendedModels: ['deepseek-chat'],
    docsUrl: 'https://api-docs.deepseek.com/',
  },
  {
    id: 'gemini-api',
    label: 'Gemini API',
    providerLabel: 'Google Gemini',
    platform: 'gemini',
    defaultUrl: 'https://generativelanguage.googleapis.com',
    recommendedSkipModelFetch: false,
    recommendedModels: [],
    docsUrl: '',
  },
  {
    id: 'openai-api',
    label: 'OpenAI API',
    providerLabel: 'OpenAI',
    platform: 'openai',
    defaultUrl: 'https://api.openai.com',
    recommendedSkipModelFetch: false,
    recommendedModels: [],
    docsUrl: '',
  },
]
let listedPresets = initializationPresets

function displayPreset(
  id: string,
  providerLabel: string,
  platform = 'openai'
): SiteInitializationPreset {
  return {
    id,
    providerLabel,
    platform,
    label: `${providerLabel} / ${platform === 'claude' ? 'Claude' : 'OpenAI'}`,
    defaultUrl: `https://example.com/${id}`,
    recommendedSkipModelFetch: false,
    recommendedModels: [],
    docsUrl: '',
  }
}

const { mockCreateMutate, mockUpdateMutate, mockDetectMutate, mockToastError } =
  vi.hoisted(() => ({
    mockCreateMutate: vi.fn(),
    mockUpdateMutate: vi.fn(),
    mockDetectMutate: vi.fn(),
    mockToastError: vi.fn(),
  }))

vi.mock('../../api', () => ({
  useSiteInitializationPresets: () => ({
    data: listedPresets,
    isPending: false,
    isError: false,
  }),
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

const originalScrollIntoView = HTMLElement.prototype.scrollIntoView

beforeAll(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
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

afterAll(() => {
  vi.unstubAllGlobals()
  HTMLElement.prototype.scrollIntoView = originalScrollIntoView
})

beforeEach(() => {
  listedPresets = initializationPresets
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

describe('SiteFormSheet layout and connection presets', () => {
  it('localizes brand names while retaining original labels and URLs for search', async () => {
    listedPresets = [
      displayPreset('bailian', 'Alibaba Bailian'),
      displayPreset('codingplan-openai', 'Aliyun CodingPlan'),
      displayPreset('moonshot-openai', 'Moonshot / Kimi'),
    ]
    await i18n.changeLanguage('zhCN')
    try {
      render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
      const bailian = await screen.findByRole('button', {
        name: '使用 Alibaba Bailian / OpenAI 模板',
      })
      expect(within(bailian).getByText('阿里云百炼')).toBeInTheDocument()
      expect(screen.getByText('百炼 Coding Plan')).toBeInTheDocument()
      const search = screen.getByRole('textbox', { name: '搜索全部连接模板…' })
      for (const query of [
        '阿里云百炼',
        'Alibaba Bailian',
        'example.com/bailian',
      ]) {
        fireEvent.change(search, { target: { value: query } })
        expect(
          await screen.findByRole('button', {
            name: '使用 Alibaba Bailian / OpenAI 模板',
          })
        ).toBeInTheDocument()
      }
      fireEvent.change(search, { target: { value: 'Moonshot' } })
      expect(screen.getByText('Kimi')).toBeInTheDocument()
      await act(async () => {
        await i18n.changeLanguage('en')
      })
      fireEvent.change(search, { target: { value: 'Alibaba Bailian' } })
      expect(screen.getByText('Alibaba Cloud Bailian')).toBeInTheDocument()
    } finally {
      cleanup()
      await i18n.changeLanguage('en')
    }
  })

  it('groups Coding Plan brands and keeps Chat and Messages adjacent regardless of backend order', async () => {
    listedPresets = [
      displayPreset('zai-coding-plan-openai', 'Z.ai Coding Plan'),
      displayPreset('doubao-coding-claude', 'Doubao Coding Plan', 'claude'),
      displayPreset('minimax-claude', 'MiniMax', 'claude'),
      displayPreset('xiaomi-token-plan-claude', 'Xiaomi Token Plan', 'claude'),
      displayPreset('codingplan-openai', 'Aliyun CodingPlan'),
      displayPreset('zhipu-coding-plan-openai', 'Zhipu Coding Plan'),
      displayPreset('kimi-coding-openai', 'Kimi Coding Plan'),
      displayPreset('doubao-coding-openai', 'Doubao Coding Plan'),
      displayPreset('minimax-openai', 'MiniMax'),
    ]
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.click(await screen.findByRole('tab', { name: 'Coding Plan' }))
    const buttons = within(
      screen.getByRole('tabpanel', { name: 'Coding Plan' })
    ).getAllByRole('button')
    const expected = [
      'Aliyun CodingPlan / OpenAI',
      'Zhipu Coding Plan / OpenAI',
      'Kimi Coding Plan / OpenAI',
      'Doubao Coding Plan / OpenAI',
      'Doubao Coding Plan / Claude',
      'MiniMax / OpenAI',
      'MiniMax / Claude',
      'Xiaomi Token Plan / Claude',
      'Z.ai Coding Plan / OpenAI',
    ]
    expect(buttons).toHaveLength(expected.length)
    expected.forEach((label, index) =>
      expect(buttons[index]).toHaveAccessibleName(`Use ${label} template`)
    )
    fireEvent.click(screen.getByRole('tab', { name: 'APIs' }))
    expect(
      within(screen.getByRole('tabpanel', { name: 'APIs' })).getAllByRole(
        'button'
      )
    ).toHaveLength(2)
  })

  it('starts with New API and common domestic services, while all gateways and other services remain discoverable', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    const common = await screen.findByRole('tabpanel', { name: 'Common' })
    const commonTemplates = within(common).getAllByRole('button')
    expect(commonTemplates[0]).toHaveAccessibleName('Use New API template')
    expect(commonTemplates[1]).toHaveAccessibleName(
      'Use Aliyun CodingPlan / OpenAI template'
    )
    expect(commonTemplates[2]).toHaveAccessibleName(
      'Use DeepSeek / OpenAI template'
    )
    expect(
      screen.queryByRole('button', { name: 'Use OpenAI API template' })
    ).not.toBeInTheDocument()
    fireEvent.change(
      screen.getByRole('textbox', { name: 'Search all templates…' }),
      { target: { value: 'OpenAI API' } }
    )
    expect(
      await screen.findByRole('button', { name: 'Use OpenAI API template' })
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: 'Gateways' }))
    expect(
      within(screen.getByRole('tabpanel', { name: 'Gateways' })).getAllByRole(
        'button'
      )
    ).toHaveLength(8)
    fireEvent.click(screen.getByRole('tab', { name: 'Coding Plan' }))
    const coding = screen.getByRole('tabpanel', { name: 'Coding Plan' })
    expect(
      within(coding).getByRole('button', {
        name: 'Use Xiaomi Token Plan / Claude template',
      })
    ).toBeInTheDocument()
    expect(
      within(coding).queryByRole('button', { name: 'Use OpenAI API template' })
    ).not.toBeInTheDocument()
  })

  it('keeps paired controls top-aligned and scrolls fields independently of the header and actions', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    const name = await screen.findByLabelText('Name')
    const dialog = screen.getByRole('dialog')
    const body = dialog.querySelector('[data-slot="site-form-body"]')
    expect(body).toHaveClass('min-h-0', 'overflow-y-auto')
    expect(dialog).toHaveClass('overflow-hidden')
    expect(body).not.toContainElement(
      dialog.querySelector('[data-slot="sheet-header"]')
    )
    expect(body).not.toContainElement(
      dialog.querySelector('[data-slot="sheet-footer"]')
    )
    for (const control of [name, screen.getByLabelText('Global weight')]) {
      expect(
        control.closest('[data-slot="form-item"]')?.parentElement
      ).toHaveClass('items-start', 'sm:grid-cols-2')
    }
    expect(
      screen.getByRole('group', { name: 'Connection details' })
    ).toContainElement(name)
    expect(
      screen.getByRole('group', { name: 'Routing and probes' })
    ).toContainElement(screen.getByLabelText('Max concurrency'))
    expect(
      screen.getByRole('group', { name: 'Proxy and requests' })
    ).toContainElement(screen.getByLabelText('Proxy URL'))
  })

  it('searches service presets and fills a blank connection with a canonical adapter', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.change(
      await screen.findByRole('textbox', { name: 'Search all templates…' }),
      { target: { value: 'DeepSeek' } }
    )
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Use DeepSeek / OpenAI template',
      })
    )
    expect(screen.getByLabelText('Name')).toHaveValue('DeepSeek')
    expect(screen.getByLabelText('URL')).toHaveValue(
      'https://api.deepseek.com/v1'
    )
    expect(
      screen.getByRole('combobox', { name: 'Platform' })
    ).toHaveTextContent('OpenAI')
  })

  it('preserves a typed name and URL when applying a compatible service preset', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    await screen.findByLabelText('Name')
    typeField('Name', 'My existing connection')
    typeField('URL', 'https://gateway.example.com')
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Use DeepSeek / OpenAI template',
      })
    )
    expect(screen.getByLabelText('Name')).toHaveValue('My existing connection')
    expect(screen.getByLabelText('URL')).toHaveValue(
      'https://gateway.example.com'
    )
    expect(
      screen.getByRole('combobox', { name: 'Platform' })
    ).toHaveTextContent('OpenAI')
  })
})

describe('SiteFormSheet template changes', () => {
  it('submits the server preset ID after URL normalization, including a custom name', async () => {
    mockCreateMutate.mockResolvedValue({
      id: 42,
      name: 'My connection',
      url: 'https://api.deepseek.com',
      platform: 'openai',
    })
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Use DeepSeek / OpenAI template',
      })
    )
    typeField('Name', 'My connection')
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))
    await waitFor(() => expect(mockCreateMutate).toHaveBeenCalledTimes(1))
    expect(mockCreateMutate.mock.calls[0]?.[0]).toMatchObject({
      name: 'My connection',
      url: 'https://api.deepseek.com',
      platform: 'openai',
      initializationPresetId: 'deepseek-openai',
    })
  })

  it.each(['URL', 'Platform'])(
    'clears the preset ID after manually changing %s',
    async (field) => {
      mockCreateMutate.mockResolvedValue({
        id: 42,
        name: 'Custom',
        url: 'https://custom.example.com',
        platform: 'openai',
      })
      render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
      fireEvent.click(
        await screen.findByRole('button', {
          name: 'Use DeepSeek / OpenAI template',
        })
      )
      if (field === 'URL') typeField('URL', 'https://custom.example.com')
      else {
        fireEvent.click(screen.getByRole('combobox', { name: 'Platform' }))
        fireEvent.click(
          await screen.findByRole('option', { name: 'Anthropic Claude' })
        )
      }
      fireEvent.click(screen.getByRole('button', { name: 'Create' }))
      await waitFor(() => expect(mockCreateMutate).toHaveBeenCalledTimes(1))
      expect(mockCreateMutate.mock.calls[0]?.[0]).not.toHaveProperty(
        'initializationPresetId'
      )
    }
  )

  it('changes untouched defaults and preserves subsequent manual edits', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Use DeepSeek / OpenAI template',
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Change template' }))
    fireEvent.click(screen.getByRole('tab', { name: 'APIs' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Use Gemini API template' })
    )
    expect(screen.getByLabelText('Name')).toHaveValue('Google Gemini')
    expect(screen.getByLabelText('URL')).toHaveValue(
      'https://generativelanguage.googleapis.com'
    )
    expect(
      screen.getByRole('combobox', { name: 'Platform' })
    ).toHaveTextContent('Google Gemini')
    typeField('Name', 'My gateway')
    typeField('URL', 'https://gateway.example.com')
    fireEvent.click(screen.getByRole('button', { name: 'Change template' }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Use OpenAI API template' })
    )
    expect(screen.getByLabelText('Name')).toHaveValue('My gateway')
    expect(screen.getByLabelText('URL')).toHaveValue(
      'https://gateway.example.com'
    )
    expect(
      screen.getByRole('combobox', { name: 'Platform' })
    ).toHaveTextContent('OpenAI')
    expect(screen.getByRole('status')).toHaveTextContent(
      'Custom connection; your entries were preserved.'
    )
  })

  it('switches to a management template without retaining the previous service URL', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.click(
      await screen.findByRole('button', {
        name: 'Use DeepSeek / OpenAI template',
      })
    )
    fireEvent.click(screen.getByRole('button', { name: 'Change template' }))
    fireEvent.click(screen.getByRole('tab', { name: 'Gateways' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Use New API template' })
    )
    expect(screen.getByLabelText('Name')).toHaveValue('New API')
    expect(screen.getByLabelText('URL')).toHaveValue('')
    expect(
      screen.getByRole('combobox', { name: 'Platform' })
    ).toHaveTextContent('New API')
  })

  it('opens the existing OAuth page and keeps unavailable providers disabled', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    fireEvent.click(await screen.findByRole('tab', { name: 'OAuth' }))
    const oauth = await screen.findByRole('link', {
      name: 'Open OAuth connections for OpenAI Codex',
    })
    expect(oauth).toHaveAttribute('href', '/oauth')
    expect(oauth).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('button', { name: 'Antigravity is not available' })
    ).toBeDisabled()
    expect(mockCreateMutate).not.toHaveBeenCalled()
  })
})

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
  // Human-facing names stay readable while submissions retain canonical IDs.
  const CANONICAL_PLATFORMS = [
    'OpenAI',
    'OpenAI Codex',
    'Anthropic Claude',
    'Google Gemini',
    'Gemini CLI',
    'Antigravity',
    'xAI Grok',
    'CLIProxyAPI',
    'AnyRouter',
    'Done Hub',
    'One Hub',
    'Veloera',
    'New API',
    'Sub2API',
    'One API',
  ]

  it('lists selectable platforms with readable names and excludes SenseTime', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)

    const platformSelect = await screen.findByRole('combobox', {
      name: 'Platform',
    })

    fireEvent.click(platformSelect)

    for (const platform of CANONICAL_PLATFORMS) {
      expect(
        await screen.findByRole('option', { name: platform })
      ).toBeInTheDocument()
    }
    expect(
      screen.queryByRole('option', { name: 'SenseTime' })
    ).not.toBeInTheDocument()
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

    fireEvent.click(await screen.findByRole('combobox', { name: 'Platform' }))
    const claudeOption = await screen.findByRole('option', {
      name: 'Anthropic Claude',
    })
    fireEvent.pointerDown(claudeOption)
    fireEvent.click(claudeOption)

    await waitFor(() => {
      expect(
        screen.getByRole('combobox', { name: 'Platform' })
      ).toHaveTextContent('Anthropic Claude')
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

describe('SiteFormSheet custom headers field wiring (#1132)', () => {
  // Fill-only semantics and the read-only example data contract are owned by
  // `__tests__/custom-headers-field.test.tsx`. What only the real sheet can
  // prove is the wiring: the extracted component sits inside `<FormControl>`
  // without losing the label association, and an edited site's stored headers
  // reach it already locked.

  it('keeps the textarea labelled and its aria-describedby uniquely resolvable', async () => {
    render(<SiteFormSheet open onOpenChange={vi.fn()} editingSite={null} />)
    // The sheet renders through a portal, so the ids must be resolved against
    // document.body — `container` would see zero elements and pass vacuously.
    const scope = document.body

    // `getByLabelText` resolving at all proves FormControl's cloned
    // id/aria-describedby survived the trip through the extracted component.
    const addHeader = await screen.findByLabelText('Custom headers')
    expect(addHeader).toHaveAccessibleName(/Custom headers/)
    fireEvent.click(screen.getByRole('button', { name: 'Edit JSON' }))
    const headers = screen.getByLabelText('Custom headers')
    expect(headers.tagName).toBe('TEXTAREA')

    const describedBy = headers.getAttribute('aria-describedby') ?? ''
    expect(describedBy).not.toBe('')
    for (const id of describedBy.split(/\s+/).filter(Boolean)) {
      // A second <FormDescription> in one FormItem would emit a second element
      // carrying the same formDescriptionId — the defect this guards.
      expect(
        scope.querySelectorAll(`#${id}`),
        `aria-describedby "${id}" must resolve to exactly one element`
      ).toHaveLength(1)
    }
  })

  it("locks an existing site's stored headers end to end", async () => {
    const stored = '{"User-Agent":"claude-cli/1.2.3"}'
    render(
      <SiteFormSheet
        open
        onOpenChange={vi.fn()}
        editingSite={{
          id: 11,
          name: 'Existing site',
          url: 'https://existing.example',
          platform: 'claude',
          status: 'active',
          customHeaders: stored,
        }}
      />
    )

    const headerKey = await screen.findByLabelText('Custom headers')
    await waitFor(() => expect(headerKey).toHaveValue('User-Agent'))
    expect(screen.getByLabelText('Value')).toHaveValue('claude-cli/1.2.3')
    fireEvent.click(screen.getByRole('button', { name: 'Edit JSON' }))
    const headers = screen.getByLabelText('Custom headers')
    expect(headers).toHaveValue(stored)

    for (const client of ['Claude Code', 'Codex CLI', 'Gemini CLI']) {
      const button = screen.getByRole('button', {
        name: `Insert ${client} example`,
      })
      expect(button).toBeDisabled()
      fireEvent.click(button)
    }
    expect(headers).toHaveValue(stored)
  })
})

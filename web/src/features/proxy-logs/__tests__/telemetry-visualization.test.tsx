import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { AttemptWaterfall } from '../components/attempt-waterfall'
import { BillingSection } from '../components/proxy-log-detail-sheet'
import { RelayBridgeTraceCard } from '../components/relay-bridge-trace'
import { StreamTimingCard } from '../components/stream-timing-card'
import type {
  ProxyLogBillingDetails,
  ProxyLogConversion,
  ProxyLogStreamTiming,
} from '../types'

afterEach(() => {
  cleanup()
})

function makeStreamTiming(
  overrides: Partial<ProxyLogStreamTiming> = {}
): ProxyLogStreamTiming {
  return {
    headerMs: 120,
    firstReasoningMs: 260,
    firstTextMs: 520,
    lastContentMs: 1420,
    maxContentGapMs: 185,
    reasoningEvidence: 'visible',
    decodeTokens: 180,
    decodeWindowMs: 900,
    decodeTps: 64.5,
    decodeStatus: 'measured',
    ...overrides,
  }
}

function makeBilling(
  overrides: Partial<NonNullable<ProxyLogBillingDetails>> = {}
): NonNullable<ProxyLogBillingDetails> {
  return {
    quotaType: 0,
    usage: {
      promptTokens: 2000,
      completionTokens: 500,
      totalTokens: 2500,
      cacheReadTokens: 300,
      cacheCreationTokens: 1000,
      billablePromptTokens: 1700,
      promptTokensIncludeCache: true,
    },
    pricing: {
      modelRatio: 1,
      completionRatio: 3,
      cacheRatio: 0.1,
      cacheCreationRatio: 1.25,
      groupRatio: 1,
    },
    breakdown: {
      inputPerMillion: 3,
      outputPerMillion: 15,
      cacheReadPerMillion: 0.3,
      cacheCreationPerMillion: 3.75,
      inputCost: 0.0051,
      outputCost: 0.0075,
      cacheReadCost: 0.00009,
      cacheCreationCost: 0.00375,
      totalCost: 0.01644,
    },
    ...overrides,
  }
}

describe('StreamTimingCard and upgraded AttemptWaterfall', () => {
  it('renders 4-stage waterfall, measured Decode TPS with tooltip, and max content gap', () => {
    const timing = makeStreamTiming()
    const { container } = render(
      <>
        <AttemptWaterfall
          latencyMs={1420}
          firstByteLatencyMs={120}
          firstOutputLatencyMs={260}
          isStream
          streamTiming={timing}
          scaleMaxMs={2000}
        />
        <StreamTimingCard streamTiming={timing} latencyMs={1420} />
      </>
    )

    expect(
      container.querySelector('[data-waterfall-mode="stream-timing"]')
    ).not.toBeNull()
    expect(container.querySelectorAll('[data-stage="header"]').length).toBe(2)
    expect(container.querySelectorAll('[data-stage="reasoning"]').length).toBe(
      2
    )
    expect(container.querySelectorAll('[data-stage="textWait"]').length).toBe(2)
    expect(container.querySelectorAll('[data-stage="stream"]').length).toBe(2)

    const tps = screen.getByTestId('stream-timing-tps')
    expect(tps).toHaveTextContent('64.5 tok/s')
    expect(tps).toHaveAttribute('title', '180 tokens across 900ms')
    expect(screen.getByTestId('attempt-waterfall-tps')).toHaveTextContent(
      '64.5 tok/s'
    )
    expect(screen.getByTestId('stream-timing-max-gap')).toHaveTextContent(
      '185ms'
    )
    expect(screen.getByText('Visible reasoning')).toBeInTheDocument()
  })

  it.each([
    ['bufferedBurst', 'warning', 'Buffered burst (speed indicative)'],
    ['hiddenReasoning', 'info', 'Hidden reasoning (reasoning excluded)'],
    ['interrupted', 'destructive', 'Stream interrupted'],
    ['insufficientEvidence', 'neutral', 'Insufficient evidence'],
  ] as const)(
    'hides measured TPS and renders dual-channel %s status badge (%s)',
    (decodeStatus, expectedTone, expectedLabel) => {
      const timing = makeStreamTiming({
        decodeStatus,
        decodeTps: 99.9,
        maxContentGapMs: null,
        reasoningEvidence:
          decodeStatus === 'hiddenReasoning' ? 'hidden' : 'none',
      })
      const { container } = render(
        <StreamTimingCard streamTiming={timing} latencyMs={1420} />
      )

      expect(screen.queryByTestId('stream-timing-tps')).not.toBeInTheDocument()
      expect(
        screen.getByTestId('stream-timing-tps-unavailable')
      ).toHaveTextContent('Unknown')
      expect(screen.getByText(expectedLabel)).toBeInTheDocument()
      const badge = container.querySelector(`[data-tone="${expectedTone}"]`)
      expect(badge).not.toBeNull()
      expect(badge).toHaveTextContent(expectedLabel)
      expect(
        screen.queryByTestId('stream-timing-max-gap')
      ).not.toBeInTheDocument()
    }
  )

  it('renders a single-row compact bar without duplicate stage legend when compact is true', () => {
    const timing = makeStreamTiming()
    const { container } = render(
      <AttemptWaterfall
        latencyMs={1420}
        firstByteLatencyMs={120}
        firstOutputLatencyMs={260}
        isStream
        streamTiming={timing}
        scaleMaxMs={2000}
        compact
      />
    )
    expect(
      container.querySelector('[data-waterfall-compact="true"]')
    ).not.toBeNull()
    expect(container.querySelectorAll('[data-stage]').length).toBe(4)
    expect(
      screen.queryByTestId('attempt-waterfall-legend')
    ).not.toBeInTheDocument()
    expect(screen.getByTestId('attempt-waterfall-tps')).toHaveTextContent(
      '64.5 tok/s'
    )
    expect(screen.queryByText(/Max gap/)).not.toBeInTheDocument()
  })

  it('falls back cleanly to the basic waterfall when streamTiming is null', () => {
    const { container } = render(
      <AttemptWaterfall
        latencyMs={300}
        firstByteLatencyMs={60}
        firstOutputLatencyMs={150}
        isStream
        streamTiming={null}
        scaleMaxMs={500}
      />
    )
    expect(
      container.querySelector('[data-waterfall-mode="stream-timing"]')
    ).toBeNull()
    expect(screen.getByText(/TTFB \(headers\): 60ms/)).toBeInTheDocument()
    expect(screen.getByText(/First output: 150ms/)).toBeInTheDocument()
    expect(screen.getByText(/Stream \/ body: 150ms/)).toBeInTheDocument()
  })
})

describe('RelayBridgeTraceCard', () => {
  it('renders a restrained single-line passthrough notice for native owner without diagnostics', () => {
    const conversion: ProxyLogConversion = {
      sourceProtocol: 'anthropic_messages',
      upstreamProtocol: 'anthropic_messages',
      owner: 'native',
      quality: 'native',
      diagnostics: [],
    }
    render(<RelayBridgeTraceCard conversion={conversion} />)
    expect(screen.getByTestId('relay-bridge-native')).toBeInTheDocument()
    expect(
      screen.getByText(/Native passthrough · no protocol conversion overhead/)
    ).toBeInTheDocument()
    expect(screen.getByText('Anthropic Messages')).toBeInTheDocument()
  })

  it.each([
    ['good', 'success', 'Good fidelity'],
    ['fair', 'info', 'Fair fidelity'],
    ['discouraged', 'warning', 'Discouraged bridge'],
  ] as const)(
    'renders cross-protocol step chain, owner tag, and %s quality badge',
    (quality, expectedTone, expectedLabel) => {
      const conversion: ProxyLogConversion = {
        sourceProtocol: 'anthropic_messages',
        upstreamProtocol: 'gemini',
        owner: 'relaykit',
        quality,
        requestSteps: ['anthropic_messages', 'openai_chat', 'gemini'],
        responseSteps: ['gemini', 'anthropic_messages'],
        diagnostics: [],
      }
      const { container } = render(
        <RelayBridgeTraceCard conversion={conversion} />
      )
      expect(screen.getByTestId('relay-bridge-card')).toBeInTheDocument()
      expect(screen.getByTestId('relay-bridge-owner')).toHaveTextContent(
        'RelayKit'
      )
      expect(screen.getByTestId('relay-bridge-chain')).toHaveTextContent(
        'Anthropic MessagesOpenAI ChatGemini'
      )
      const qualityBadge = container.querySelector(
        `[data-tone="${expectedTone}"]`
      )
      expect(qualityBadge).toHaveTextContent(expectedLabel)
    }
  )

  it('renders whitelisted diagnostics as plain text with dual-channel severity badges', () => {
    const conversion: ProxyLogConversion = {
      sourceProtocol: 'openai_responses',
      upstreamProtocol: 'anthropic_messages',
      owner: 'specialized',
      quality: 'fair',
      diagnostics: [
        {
          phase: 'request',
          code: 'dropped_field',
          severity: 'warning',
          fieldClass: 'metadata',
        },
        {
          phase: 'response',
          code: '<script>unsupported_stop</script>',
          severity: 'error',
          fieldClass: 'finish_reason',
        },
      ],
    }
    const { container } = render(
      <RelayBridgeTraceCard conversion={conversion} />
    )
    expect(screen.getByTestId('relay-bridge-diagnostics')).toBeInTheDocument()
    expect(screen.getByText('dropped_field')).toBeInTheDocument()
    expect(
      screen.getByText('<script>unsupported_stop</script>')
    ).toBeInTheDocument()
    expect(container.querySelector('script')).toBeNull()
    expect(screen.getByText('request · metadata')).toBeInTheDocument()
    expect(screen.getByText('response · finish_reason')).toBeInTheDocument()
    expect(container.querySelector('[data-tone="warning"]')).not.toBeNull()
    expect(container.querySelector('[data-tone="destructive"]')).not.toBeNull()
  })
})

describe('BillingSection tiered cache writes and tool surcharges', () => {
  it('expands 5m/1h/12h cache creation tiers with standard remainder and omits zero tiers', () => {
    const billing = makeBilling({
      usage: {
        promptTokens: 2000,
        completionTokens: 500,
        totalTokens: 2500,
        cacheReadTokens: 300,
        cacheCreationTokens: 1600,
        cacheCreation5mTokens: 400,
        cacheCreation1hTokens: 0,
        cacheCreation12hTokens: 900,
        billablePromptTokens: 1700,
        promptTokensIncludeCache: true,
      },
      breakdown: {
        inputPerMillion: 3,
        outputPerMillion: 15,
        cacheReadPerMillion: 0.3,
        cacheCreationPerMillion: 3.75,
        inputCost: 0.0051,
        outputCost: 0.0075,
        cacheReadCost: 0.00009,
        cacheCreationCost: 0.011,
        cacheCreation5mCost: 0.0015,
        cacheCreation1hCost: 0,
        cacheCreation12hCost: 0.0075,
        totalCost: 0.02369,
      },
    })

    render(<BillingSection billing={billing} />)

    expect(screen.getByText('Cache write (5m)')).toBeInTheDocument()
    expect(screen.getByText('Cache write (12h)')).toBeInTheDocument()
    expect(screen.getByText('Cache write (standard)')).toBeInTheDocument()
    expect(screen.queryByText('Cache write (1h)')).not.toBeInTheDocument()
    expect(screen.getByText('400')).toBeInTheDocument()
    expect(screen.getByText('900')).toBeInTheDocument()
    // Remainder: 1600 - (400 + 900) = 300 (also matches cacheReadTokens)
    expect(screen.getByText('$0.002000')).toBeInTheDocument()
  })

  it('renders managed tool surcharges including missing price and free $0 edge cases', () => {
    const billing = makeBilling({
      breakdown: {
        inputPerMillion: 3,
        outputPerMillion: 15,
        cacheReadPerMillion: 0.3,
        cacheCreationPerMillion: 3.75,
        inputCost: 0.0051,
        outputCost: 0.0075,
        cacheReadCost: 0.00009,
        cacheCreationCost: 0.00375,
        toolSurcharges: [
          {
            tool: 'web_search',
            unit: 'queries',
            quantity: 3,
            source: 'vendor_reported',
            unitPrice: 0.01,
            cost: 0.03,
          },
          {
            tool: 'grounding',
            unit: 'queries',
            quantity: 2,
            source: 'response_derived',
            unitPrice: null,
            cost: 0,
          },
          {
            tool: 'file_search',
            unit: 'calls',
            quantity: 1,
            unitPrice: 0,
            cost: 0,
          },
        ],
        totalCost: 0.04644,
      },
    })

    render(<BillingSection billing={billing} />)

    expect(screen.getByTestId('billing-tool-surcharges')).toBeInTheDocument()
    expect(screen.getByText('Managed tool surcharges')).toBeInTheDocument()
    expect(screen.getByText('Web search')).toBeInTheDocument()
    expect(screen.getByText('Grounding')).toBeInTheDocument()
    expect(screen.getByText('File search')).toBeInTheDocument()
    expect(screen.getByText('3 queries')).toBeInTheDocument()
    expect(screen.getByText('2 queries')).toBeInTheDocument()
    expect(screen.getByText('1 calls')).toBeInTheDocument()
    expect(screen.getAllByText('Provider reported')).toHaveLength(1)
    expect(screen.getByText('Response inferred')).toBeInTheDocument()
    expect(screen.getByText('$0.010000')).toBeInTheDocument()
    expect(screen.getByText('Unpriced')).toBeInTheDocument()
    expect(screen.getByText('Free $0')).toBeInTheDocument()
  })

  it('formats backend chat/messages protocol names consistently in RelayBridgeTraceCard', () => {
    render(
      <RelayBridgeTraceCard
        conversion={{
          owner: 'relaykit',
          sourceProtocol: 'chat',
          upstreamProtocol: 'messages',
          quality: 'good',
          diagnostics: [],
        }}
      />
    )
    expect(screen.getByText('OpenAI Chat')).toBeInTheDocument()
    expect(screen.getByText('Anthropic Messages')).toBeInTheDocument()
  })

  it('keeps legacy billing objects working without tiered cache or tool surcharge fields', () => {
    const legacyBilling = makeBilling()
    render(<BillingSection billing={legacyBilling} />)

    expect(screen.getByText('Cache write')).toBeInTheDocument()
    expect(screen.queryByText('Cache write (5m)')).not.toBeInTheDocument()
    expect(
      screen.queryByTestId('billing-tool-surcharges')
    ).not.toBeInTheDocument()
    expect(screen.getByText('$0.016440')).toBeInTheDocument()
  })
})

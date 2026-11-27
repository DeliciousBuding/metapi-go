// Unit + component tests for ProtocolReachabilityPills: 4-protocol matrix
// evaluation (all native, single Chat direct bridge, Responses direct bridge,
// Claude Messages + Gemini multi-hop bridge, empty endpoints) and bitmask /
// endpointConfig extraction.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  collectConfiguredConversationProtocols,
  evaluateProtocolReachability,
  ProtocolReachabilityPills,
} from '../components/protocol-reachability-pills'

afterEach(() => cleanup())

describe('evaluateProtocolReachability', () => {
  it('marks all four protocols as native when all are configured', () => {
    expect(
      evaluateProtocolReachability(['chat', 'responses', 'messages', 'gemini'])
    ).toEqual([
      { protocol: 'chat', state: 'native', bridgeTarget: null },
      { protocol: 'responses', state: 'native', bridgeTarget: null },
      { protocol: 'messages', state: 'native', bridgeTarget: null },
      { protocol: 'gemini', state: 'native', bridgeTarget: null },
    ])
  })

  it('bridges Responses, Messages, and Gemini directly when only Chat is configured', () => {
    expect(evaluateProtocolReachability(['chat'])).toEqual([
      { protocol: 'chat', state: 'native', bridgeTarget: null },
      { protocol: 'responses', state: 'direct_bridge', bridgeTarget: 'chat' },
      { protocol: 'messages', state: 'direct_bridge', bridgeTarget: 'chat' },
      { protocol: 'gemini', state: 'direct_bridge', bridgeTarget: 'chat' },
    ])
  })

  it('distinguishes Chat ↔ Responses direct bridge from Messages/Gemini multi-hop when only Responses is configured', () => {
    expect(evaluateProtocolReachability(['responses'])).toEqual([
      { protocol: 'chat', state: 'direct_bridge', bridgeTarget: 'responses' },
      { protocol: 'responses', state: 'native', bridgeTarget: null },
      {
        protocol: 'messages',
        state: 'multi_hop_bridge',
        bridgeTarget: 'responses',
      },
      {
        protocol: 'gemini',
        state: 'multi_hop_bridge',
        bridgeTarget: 'responses',
      },
    ])
  })

  it('marks Claude Messages → Gemini as multi-hop when only Messages is configured', () => {
    expect(evaluateProtocolReachability(['messages'])).toEqual([
      { protocol: 'chat', state: 'direct_bridge', bridgeTarget: 'messages' },
      {
        protocol: 'responses',
        state: 'multi_hop_bridge',
        bridgeTarget: 'messages',
      },
      { protocol: 'messages', state: 'native', bridgeTarget: null },
      {
        protocol: 'gemini',
        state: 'multi_hop_bridge',
        bridgeTarget: 'messages',
      },
    ])
  })

  it('keeps Messages and Gemini native while marking Responses multi-hop for a Messages+Gemini upstream', () => {
    expect(evaluateProtocolReachability(['messages', 'gemini'])).toEqual([
      { protocol: 'chat', state: 'direct_bridge', bridgeTarget: 'messages' },
      {
        protocol: 'responses',
        state: 'multi_hop_bridge',
        bridgeTarget: 'messages',
      },
      { protocol: 'messages', state: 'native', bridgeTarget: null },
      { protocol: 'gemini', state: 'native', bridgeTarget: null },
    ])
  })

  it('marks all four protocols unavailable when no conversational endpoint is configured', () => {
    expect(evaluateProtocolReachability([])).toEqual([
      { protocol: 'chat', state: 'unavailable', bridgeTarget: null },
      { protocol: 'responses', state: 'unavailable', bridgeTarget: null },
      { protocol: 'messages', state: 'unavailable', bridgeTarget: null },
      { protocol: 'gemini', state: 'unavailable', bridgeTarget: null },
    ])
  })
})

describe('collectConfiguredConversationProtocols', () => {
  it('merges endpointConfig keys and protocol bitmask while ignoring non-conversational bits', () => {
    const result = collectConfiguredConversationProtocols({
      endpointConfig: {
        chat: {
          url: 'https://api.example/v1/chat/completions',
          auth: 'bearer',
        },
        embeddings: {
          url: 'https://api.example/v1/embeddings',
          auth: 'bearer',
        },
      },
      // 8 = messages, 64 = embeddings (ignored)
      protocolsBitmask: 8 | 64,
    })
    expect(result).toEqual(['chat', 'messages'])
  })

  it('honors explicit protocolOrder over raw bitmask when provided', () => {
    const result = collectConfiguredConversationProtocols({
      protocolsBitmask: 2 | 4 | 8 | 16,
      protocolOrder: [8, 16],
    })
    expect(result).toEqual(['messages', 'gemini'])
  })
})

function getProtocolPills() {
  const group = screen.getByRole('group', { name: 'Protocol endpoints' })
  return Array.from(group.querySelectorAll<HTMLElement>('[data-protocol]'))
}

describe('ProtocolReachabilityPills component', () => {
  it('renders 4 pills with native solid states when all endpoints exist', () => {
    render(
      <ProtocolReachabilityPills
        configured={['chat', 'responses', 'messages', 'gemini']}
      />
    )

    const pills = getProtocolPills()
    expect(pills).toHaveLength(4)
    for (const pill of pills) {
      expect(pill).toHaveAttribute('data-reachability', 'native')
    }
    expect(pills[0]).toHaveAccessibleName('Chat: Configured')
    expect(pills[1]).toHaveAccessibleName('Responses: Configured')
    expect(pills[2]).toHaveAccessibleName('Messages: Configured')
    expect(pills[3]).toHaveAccessibleName('Gemini: Configured')
  })

  it('renders direct bridge semantics when only Chat is configured', () => {
    render(<ProtocolReachabilityPills configured={['chat']} />)

    const pills = getProtocolPills()
    expect(pills[0]).toHaveAttribute('data-reachability', 'native')
    expect(pills[1]).toHaveAttribute('data-reachability', 'direct_bridge')
    expect(pills[1]).toHaveAccessibleName('Responses: Responses → Chat')
    expect(pills[2]).toHaveAttribute('data-reachability', 'direct_bridge')
    expect(pills[3]).toHaveAttribute('data-reachability', 'direct_bridge')
  })

  it('renders multi-hop bridge semantics for Claude + Gemini combination', () => {
    render(<ProtocolReachabilityPills configured={['messages', 'gemini']} />)

    const pills = getProtocolPills()
    expect(pills[0]).toHaveAttribute('data-reachability', 'direct_bridge')
    expect(pills[1]).toHaveAttribute('data-reachability', 'multi_hop_bridge')
    expect(pills[1]).toHaveAccessibleName(
      'Responses: Responses → Chat → Messages'
    )
    expect(pills[2]).toHaveAttribute('data-reachability', 'native')
    expect(pills[3]).toHaveAttribute('data-reachability', 'native')
  })

  it('renders muted unavailable states when no endpoints are configured', () => {
    render(<ProtocolReachabilityPills configured={[]} />)

    const pills = getProtocolPills()
    expect(pills).toHaveLength(4)
    for (const pill of pills) {
      expect(pill).toHaveAttribute('data-reachability', 'unavailable')
      expect(pill.getAttribute('aria-label')).toMatch(/Endpoint not configured/)
    }
  })
})

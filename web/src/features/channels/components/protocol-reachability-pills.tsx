// metapi-go/features/channels/components — 4-protocol native / RelayKit bridge
// reachability matrix pills.
//
// Summarizes how the four conversational client protocols (Chat, Responses,
// Messages, Gemini) reach a configured upstream connection or route member:
//   - Native: endpoint is directly configured for that protocol (solid success dot
//     + solid border)
//   - Direct bridge: not configured natively, reachable through a direct bridge
//     hop (Chat ↔ Responses, or any Chat-anchored single hop) — hollow info ring
//     + dashed border
//   - Multi-hop bridge: Claude Messages ↔ Gemini without a direct Chat/Responses
//     anchor, routed via canonical IR / intermediate hop — hollow warning ring +
//     dashed border
//   - Unavailable: no convertible conversational protocol configured — muted pill

import { useTranslation } from 'react-i18next'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { ImportedEndpointConfig } from '@/lib/api/imported-upstreams'
import { cn } from '@/lib/utils'

import { upstreamProtocols } from '../lib/upstream-config'

const CONVERSATION_PROTOCOLS = [
  'chat',
  'responses',
  'messages',
  'gemini',
] as const

type ConversationProtocol = (typeof CONVERSATION_PROTOCOLS)[number]

type ProtocolReachabilityState =
  | 'native'
  | 'direct_bridge'
  | 'multi_hop_bridge'
  | 'unavailable'

type ProtocolReachabilityEntry = {
  protocol: ConversationProtocol
  state: ProtocolReachabilityState
  /** Native configured protocol that acts as the bridge target, when bridged. */
  bridgeTarget: ConversationProtocol | null
}

const MULTI_HOP_FALLBACK_ORDER: Record<
  Exclude<ConversationProtocol, 'chat'>,
  readonly ConversationProtocol[]
> = {
  responses: ['messages', 'gemini'],
  messages: ['responses', 'gemini'],
  gemini: ['responses', 'messages'],
}

const CONVERSATION_BITS: Record<ConversationProtocol, number> = {
  chat: 2,
  responses: 4,
  messages: 8,
  gemini: 16,
}

/**
 * Extract the set of configured conversational protocols from an endpoint
 * config object and/or a persisted protocol bitmask (with optional explicit
 * protocolOrder).
 */
export function collectConfiguredConversationProtocols(source: {
  endpointConfig?: ImportedEndpointConfig | null
  protocolsBitmask?: number
  protocolOrder?: readonly number[]
}): ConversationProtocol[] {
  const configured = new Set<ConversationProtocol>()

  if (source.endpointConfig) {
    for (const protocol of CONVERSATION_PROTOCOLS) {
      if (source.endpointConfig[protocol]) {
        configured.add(protocol)
      }
    }
  }

  if (source.protocolOrder && source.protocolOrder.length > 0) {
    for (const bit of source.protocolOrder) {
      const matched = upstreamProtocols.find((p) => p.bit === bit)
      if (
        matched &&
        (CONVERSATION_PROTOCOLS as readonly string[]).includes(matched.key)
      ) {
        configured.add(matched.key as ConversationProtocol)
      }
    }
  } else if (
    typeof source.protocolsBitmask === 'number' &&
    source.protocolsBitmask > 0
  ) {
    for (const protocol of CONVERSATION_PROTOCOLS) {
      if ((source.protocolsBitmask & CONVERSATION_BITS[protocol]) !== 0) {
        configured.add(protocol)
      }
    }
  }

  return CONVERSATION_PROTOCOLS.filter((protocol) => configured.has(protocol))
}

/**
 * Compute the 4-protocol RelayKit reachability matrix for a set of configured
 * native conversational endpoints.
 *
 * Matrix rules:
 * - Native: protocol is directly present in `configured`.
 * - Direct bridge (`direct_bridge`):
 *   - `chat` ↔ `responses` (direct OpenAI family pair)
 *   - `chat` ↔ `messages` or `chat` ↔ `gemini` (single hop via Chat canonical)
 *   - `responses` ↔ `messages` or `responses` ↔ `gemini` when `chat` is present
 *     (prefers `chat` as single-hop anchor)
 * - Multi-hop bridge (`multi_hop_bridge`):
 *   - `messages` ↔ `gemini` without `chat` (Claude ↔ Gemini mediated hop), or
 *     `responses` ↔ `messages` / `responses` ↔ `gemini` without `chat`.
 * - Unavailable: no conversational protocol configured.
 */
export function evaluateProtocolReachability(
  configured: readonly ConversationProtocol[]
): ProtocolReachabilityEntry[] {
  const nativeSet = new Set<ConversationProtocol>(configured)

  return CONVERSATION_PROTOCOLS.map((protocol) => {
    if (nativeSet.has(protocol)) {
      return { protocol, state: 'native', bridgeTarget: null }
    }
    if (nativeSet.size === 0) {
      return { protocol, state: 'unavailable', bridgeTarget: null }
    }

    // Prefer direct bridge targets first:
    // 1. Chat ↔ Responses pair
    if (protocol === 'chat' && nativeSet.has('responses')) {
      return { protocol, state: 'direct_bridge', bridgeTarget: 'responses' }
    }
    if (protocol === 'responses' && nativeSet.has('chat')) {
      return { protocol, state: 'direct_bridge', bridgeTarget: 'chat' }
    }

    // 2. Chat-anchored single-hop bridge for Messages / Gemini (or back to Chat)
    if (nativeSet.has('chat')) {
      return { protocol, state: 'direct_bridge', bridgeTarget: 'chat' }
    }
    if (protocol === 'chat') {
      const target: ConversationProtocol = nativeSet.has('messages')
        ? 'messages'
        : 'gemini'
      return { protocol, state: 'direct_bridge', bridgeTarget: target }
    }

    // 3. Remaining pairs without Chat (Messages ↔ Gemini, Responses ↔ Messages/Gemini)
    // require a multi-hop canonical relay.
    const fallbackOrder = MULTI_HOP_FALLBACK_ORDER[protocol]
    const bridgeTarget =
      fallbackOrder.find((candidate) => nativeSet.has(candidate)) ?? null

    if (!bridgeTarget) {
      return { protocol, state: 'unavailable', bridgeTarget: null }
    }
    return { protocol, state: 'multi_hop_bridge', bridgeTarget }
  })
}

const PILL_CLASS: Record<ProtocolReachabilityState, string> = {
  native: 'border-solid border-success/45 bg-success/10 text-success-soft-fg',
  direct_bridge: 'border-dashed border-info/55 bg-info/10 text-info-soft-fg',
  multi_hop_bridge:
    'border-dashed border-warning/60 bg-warning/10 text-warning-soft-fg',
  unavailable: 'border-solid border-border bg-muted/50 text-muted-foreground',
}

const DOT_CLASS: Record<ProtocolReachabilityState, string> = {
  native: 'bg-success',
  direct_bridge: 'border-[1.5px] border-info bg-transparent',
  multi_hop_bridge: 'border-[1.5px] border-warning bg-transparent',
  unavailable: 'bg-muted-foreground/45',
}

type ProtocolReachabilityPillsProps = {
  /** Native configured conversational protocols (chat/responses/messages/gemini). */
  configured: readonly ConversationProtocol[]
  className?: string
}

export function ProtocolReachabilityPills({
  configured,
  className,
}: ProtocolReachabilityPillsProps) {
  const { t } = useTranslation()
  const entries = evaluateProtocolReachability(configured)

  return (
    <TooltipProvider delay={150}>
      <div
        role='group'
        aria-label={t('channels.imported.endpoints')}
        className={cn('flex min-w-0 flex-wrap items-center gap-1', className)}
      >
        {entries.map((entry) => {
          const protocolName = t(
            `channels.capabilities.names.${entry.protocol}`
          )
          const targetName = entry.bridgeTarget
            ? t(`channels.capabilities.names.${entry.bridgeTarget}`)
            : null

          let stateSummary: string
          let detailHint: string
          if (entry.state === 'native') {
            stateSummary = t('channels.upstream.configured')
            detailHint = `${protocolName} · ${t('channels.upstream.configured')}`
          } else if (entry.state === 'direct_bridge') {
            stateSummary = targetName
              ? `${protocolName} → ${targetName}`
              : t('channels.capabilities.orderGroups.conversation')
            detailHint = t('tokenRoutes.preview.conversion')
          } else if (entry.state === 'multi_hop_bridge') {
            const chatName = t('channels.capabilities.names.chat')
            stateSummary = targetName
              ? `${protocolName} → ${chatName} → ${targetName}`
              : t('channels.capabilities.orderGroups.conversation')
            detailHint = t('tokenRoutes.preview.conversion')
          } else {
            stateSummary = t('channels.capabilities.notConfigured')
            detailHint = t('channels.capabilities.notConfigured')
          }

          return (
            <Tooltip key={entry.protocol}>
              <TooltipTrigger
                render={
                  <span
                    tabIndex={0}
                    data-protocol={entry.protocol}
                    data-reachability={entry.state}
                    aria-label={`${protocolName}: ${stateSummary}`}
                    className={cn(
                      'focus-visible:ring-focus-ring inline-flex h-5 shrink-0 items-center gap-1 rounded-full border px-2 text-2xs font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-2',
                      PILL_CLASS[entry.state]
                    )}
                  />
                }
              >
                <span
                  aria-hidden='true'
                  className={cn(
                    'size-1.5 shrink-0 rounded-full',
                    DOT_CLASS[entry.state]
                  )}
                />
                <span>{protocolName}</span>
              </TooltipTrigger>
              <TooltipContent
                side='top'
                className='flex-col items-start gap-0.5 text-left'
              >
                <span className='font-medium'>{stateSummary}</span>
                {detailHint !== stateSummary && (
                  <span className='opacity-85'>{detailHint}</span>
                )}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    </TooltipProvider>
  )
}

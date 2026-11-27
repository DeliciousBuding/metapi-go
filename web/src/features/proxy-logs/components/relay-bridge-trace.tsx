import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'

import type { ProxyLogConversion } from '../types'

type RelayBridgeTraceCardProps = {
  conversion: ProxyLogConversion | null | undefined
}

const PROTOCOL_LABELS: Record<string, string> = {
  chat: 'OpenAI Chat',
  openai: 'OpenAI Chat',
  openai_chat: 'OpenAI Chat',
  'openai-chat': 'OpenAI Chat',
  chat_completions: 'OpenAI Chat',
  responses: 'OpenAI Responses',
  openai_responses: 'OpenAI Responses',
  'openai-responses': 'OpenAI Responses',
  messages: 'Anthropic Messages',
  anthropic: 'Anthropic Messages',
  anthropic_messages: 'Anthropic Messages',
  'anthropic-messages': 'Anthropic Messages',
  claude: 'Anthropic Messages',
  gemini: 'Gemini',
  google_gemini: 'Gemini',
  ollama: 'Ollama',
}

function formatProtocolName(raw: string): string {
  const normalized = raw.trim().toLowerCase()
  return PROTOCOL_LABELS[normalized] ?? raw.trim()
}

function getQualityMeta(quality: string | null | undefined): {
  tone: SoftTone
  labelKey: string
} | null {
  switch (quality) {
    case 'good':
      return {
        tone: 'success',
        labelKey: 'proxyLogs.telemetry.conversion.quality.good',
      }
    case 'fair':
      return {
        tone: 'info',
        labelKey: 'proxyLogs.telemetry.conversion.quality.fair',
      }
    case 'discouraged':
      return {
        tone: 'warning',
        labelKey: 'proxyLogs.telemetry.conversion.quality.discouraged',
      }
    case 'native':
      return {
        tone: 'success',
        labelKey: 'proxyLogs.telemetry.conversion.quality.native',
      }
    default:
      return null
  }
}

function getDiagnosticSeverityMeta(severity: string): {
  tone: SoftTone
  labelKey: string
} {
  switch (severity) {
    case 'error':
      return {
        tone: 'destructive',
        labelKey: 'proxyLogs.telemetry.conversion.severity.error',
      }
    case 'warning':
      return {
        tone: 'warning',
        labelKey: 'proxyLogs.telemetry.conversion.severity.warning',
      }
    default:
      return {
        tone: 'neutral',
        labelKey: 'proxyLogs.telemetry.conversion.severity.unknown',
      }
  }
}

function getOwnerLabelKey(owner: string): string | null {
  switch (owner) {
    case 'native':
      return 'proxyLogs.telemetry.conversion.owner.native'
    case 'relaykit':
      return 'proxyLogs.telemetry.conversion.owner.relaykit'
    case 'specialized':
      return 'proxyLogs.telemetry.conversion.owner.specialized'
    default:
      return null
  }
}

function buildRouteChain(conversion: ProxyLogConversion): string[] {
  const requestSteps = (conversion.requestSteps ?? [])
    .map((step) => step.trim())
    .filter(Boolean)
  if (requestSteps.length > 0) {
    return requestSteps.map(formatProtocolName)
  }
  const endpoints = [conversion.sourceProtocol, conversion.upstreamProtocol]
    .filter((value): value is string => Boolean(value && value.trim()))
    .map(formatProtocolName)
  return endpoints
}

function keyItems<T>(
  items: T[],
  baseKey: (item: T) => string
): Array<{ item: T; key: string; isFirst: boolean }> {
  const counts = new Map<string, number>()
  return items.map((item, position) => {
    const base = baseKey(item)
    const seen = (counts.get(base) ?? 0) + 1
    counts.set(base, seen)
    return {
      item,
      key: seen === 1 ? base : `${base}#${seen}`,
      isFirst: position === 0,
    }
  })
}

export function RelayBridgeTraceCard({
  conversion,
}: RelayBridgeTraceCardProps) {
  const { t } = useTranslation()
  if (!conversion) return null

  const diagnostics = conversion.diagnostics ?? []
  const isNative = conversion.owner === 'native'

  if (isNative && diagnostics.length === 0) {
    return (
      <div
        className='bg-muted/20 flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2 text-xs'
        data-testid='relay-bridge-native'
      >
        <div className='flex items-center gap-2'>
          <SoftToneBadge tone='success'>
            {t('proxyLogs.telemetry.conversion.owner.native')}
          </SoftToneBadge>
          <span className='text-muted-foreground'>
            {t('proxyLogs.telemetry.conversion.nativePassthrough')}
          </span>
        </div>
        {conversion.sourceProtocol && (
          <span className='text-muted-foreground font-mono'>
            {formatProtocolName(conversion.sourceProtocol)}
          </span>
        )}
      </div>
    )
  }

  const chainEntries = keyItems(buildRouteChain(conversion), (step) => step)
  const responseStepEntries = keyItems(
    (conversion.responseSteps ?? [])
      .map((step) => step.trim())
      .filter(Boolean)
      .map(formatProtocolName),
    (step) => step
  )
  const diagnosticEntries = keyItems(
    diagnostics,
    (item) => `${item.phase}:${item.code}:${item.fieldClass}:${item.severity}`
  )
  const qualityMeta = getQualityMeta(conversion.quality)
  const ownerKey = conversion.owner ? getOwnerLabelKey(conversion.owner) : null

  return (
    <section
      className='bg-muted/20 space-y-2.5 rounded-lg border p-3'
      data-testid='relay-bridge-card'
    >
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h4 className='text-xs font-medium'>
          {t('proxyLogs.telemetry.conversion.title')}
        </h4>
        <div className='flex flex-wrap items-center gap-1.5'>
          {conversion.owner && (
            <span
              className='bg-muted text-muted-foreground rounded-4xl border px-1.5 py-0.5 font-mono text-xs'
              data-testid='relay-bridge-owner'
            >
              {ownerKey ? t(ownerKey) : conversion.owner}
            </span>
          )}
          {qualityMeta && (
            <SoftToneBadge tone={qualityMeta.tone}>
              {t(qualityMeta.labelKey)}
            </SoftToneBadge>
          )}
        </div>
      </div>

      {chainEntries.length > 0 && (
        <div
          className='flex flex-wrap items-center gap-1.5 text-xs'
          data-testid='relay-bridge-chain'
        >
          {chainEntries.map(({ item: step, key, isFirst }) => (
            <span key={key} className='inline-flex items-center gap-1.5'>
              {!isFirst && (
                <ArrowRight
                  className='text-muted-foreground size-3 shrink-0'
                  aria-hidden='true'
                />
              )}
              <span className='bg-card rounded border px-1.5 py-0.5 font-medium'>
                {step}
              </span>
            </span>
          ))}
        </div>
      )}

      {responseStepEntries.length > 0 && (
        <div className='text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs'>
          <span>{t('proxyLogs.telemetry.conversion.responsePath')}:</span>
          {responseStepEntries.map(({ item: step, key, isFirst }) => (
            <span key={key} className='inline-flex items-center gap-1'>
              {!isFirst && (
                <ArrowRight className='size-3 shrink-0' aria-hidden='true' />
              )}
              <span className='font-mono'>{step}</span>
            </span>
          ))}
        </div>
      )}

      {diagnosticEntries.length > 0 && (
        <div
          className='space-y-1.5 border-t pt-2'
          data-testid='relay-bridge-diagnostics'
        >
          <p className='text-muted-foreground text-xs font-medium'>
            {t('proxyLogs.telemetry.conversion.diagnosticsTitle', {
              count: diagnosticEntries.length,
            })}
          </p>
          <ul className='divide-border divide-y rounded-md border'>
            {diagnosticEntries.map(({ item, key }) => {
              const severityMeta = getDiagnosticSeverityMeta(item.severity)
              return (
                <li
                  key={key}
                  className='flex flex-wrap items-center gap-x-2 gap-y-1 px-2.5 py-1.5 text-xs'
                >
                  <SoftToneBadge tone={severityMeta.tone}>
                    {t(severityMeta.labelKey)}
                  </SoftToneBadge>
                  <span className='font-mono font-medium'>{item.code}</span>
                  <span className='text-muted-foreground'>
                    {t('proxyLogs.telemetry.conversion.diagnosticMeta', {
                      phase: item.phase,
                      fieldClass: item.fieldClass,
                    })}
                  </span>
                </li>
              )
            })}
          </ul>
        </div>
      )}
    </section>
  )
}

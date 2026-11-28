import { Link } from '@tanstack/react-router'
import { ChevronDown } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import {
  parseEventMessage,
  parsePanelPath,
  splitEnrichmentNames,
  type ProgramEvent,
} from '../lib/event-normalize'

/**
 * Structured rendering of an event message. Enriched alert messages carry
 * "Affected routes / Alternative sites / Panel" lines appended by the backend;
 * those render as labeled rows with real SPA links instead of raw text.
 *
 * The enrichment lines are collapsed behind a per-row Details toggle (#26
 * from the visual QA wave): stacking model/reason/affected-routes/
 * alternative-sites into the title column blew row heights up to ~5 lines
 * per event. The base message keeps its 2-line clamp; the operator expands
 * only the rows they need.
 */
export function EventMessage({ event }: { event: ProgramEvent }) {
  const { t } = useTranslation()
  const [expanded, setExpanded] = useState(false)
  if (!event.message) return null
  const parts = parseEventMessage(event.message)
  const routes = parts.routes ? splitEnrichmentNames(parts.routes) : []
  const sites = parts.sites ? splitEnrichmentNames(parts.sites) : []
  const panel =
    parts.panelPath !== null ? parsePanelPath(parts.panelPath) : null
  const hasEnrichment = routes.length > 0 || sites.length > 0 || panel !== null
  // Structured rows: the message renders from the `events.messages.*`
  // locale template with the event's typed params interpolated by i18next;
  // legacy rows keep the parsed-text path below.
  const structuredMessage =
    event.titleKey && event.params
      ? t(`events.messages.${event.titleKey}`, {
          defaultValue: event.message,
          ...event.params,
        })
      : null
  // Single-level ternary discipline (oxlint no-nested-ternary): compute the
  // base message element before the JSX return.
  let messageBody: ReactNode = null
  if (structuredMessage !== null) {
    messageBody = (
      <span className='line-clamp-2 break-all' title={event.message}>
        {structuredMessage}
      </span>
    )
  } else if (parts.base) {
    messageBody = (
      <span className='line-clamp-2 break-all' title={parts.base}>
        {parts.base}
      </span>
    )
  }
  return (
    <span className='text-muted-foreground flex max-w-90 flex-col gap-0.5 text-xs'>
      {messageBody}
      {expanded && routes.length > 0 ? (
        <span className='flex flex-wrap items-baseline gap-x-1'>
          <span className='shrink-0'>
            {t('settings.operations.programLogs.messageParts.affectedRoutes')}:
          </span>
          {routes.map((name, index) => (
            <span key={name} className='inline-flex items-center gap-1'>
              <Link
                to='/token-routes'
                search={{ q: name }}
                className='text-primary underline-offset-2 hover:underline'
              >
                {name}
              </Link>
              {index < routes.length - 1 ? ',' : ''}
            </span>
          ))}
        </span>
      ) : null}
      {expanded && sites.length > 0 ? (
        <span className='flex flex-wrap items-baseline gap-x-1'>
          <span className='shrink-0'>
            {t('settings.operations.programLogs.messageParts.alternativeSites')}
            :
          </span>
          {sites.map((name, index) => (
            <span key={name} className='inline-flex items-center gap-1'>
              <Link
                to='/sites'
                search={{ q: name }}
                className='text-primary underline-offset-2 hover:underline'
              >
                {name}
              </Link>
              {index < sites.length - 1 ? ',' : ''}
            </span>
          ))}
        </span>
      ) : null}
      {expanded && panel ? <PanelLink panel={panel} /> : null}
      {hasEnrichment ? (
        <button
          type='button'
          onClick={() => setExpanded((value) => !value)}
          className='text-muted-foreground hover:text-foreground focus-visible:ring-focus-ring text-2xs inline-flex w-fit items-center gap-0.5 underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:outline-none focus-visible:ring-inset'
          aria-expanded={expanded}
        >
          <ChevronDown
            aria-hidden='true'
            className={cn(
              'size-3.5 transition-transform',
              expanded && 'rotate-180'
            )}
          />
          {expanded
            ? t('settings.operations.programLogs.messageParts.collapse')
            : t('settings.operations.programLogs.messageParts.details')}
        </button>
      ) : null}
    </span>
  )
}

/** Render a parsed panel path as a typed router link (known targets only). */
function PanelLink({ panel }: { panel: ReturnType<typeof parsePanelPath> }) {
  const { t } = useTranslation()
  if (!panel) return null
  const className =
    'text-primary inline-flex items-center gap-1 underline-offset-2 hover:underline'
  const label = t('settings.operations.programLogs.messageParts.openPanel')
  switch (panel.to) {
    case '/observability':
      return (
        <Link
          to='/observability'
          search={{
            section: (panel.search.section ?? 'health') as
              | 'overview'
              | 'health',
          }}
          className={className}
        >
          {label}
        </Link>
      )
    default:
      return null
  }
}

import { Link } from '@tanstack/react-router'
import { ArrowUpRight } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import type { TrafficFlowResponse } from '@/lib/api'
import { formatInt } from '@/lib/format'

import type { FlowNode } from './flow-graph'

export function FlowSelectionDetails({
  selectedNode,
  matchingEdgeCount,
  timeWindow,
  failedOnly,
  locale,
}: {
  selectedNode: FlowNode | null
  matchingEdgeCount: number
  timeWindow?: TrafficFlowResponse['window']
  failedOnly: boolean
  locale: string
}) {
  const { t } = useTranslation()
  // Build deep-link query params for the selected node.
  const selectedLogsSearch = useMemo(() => {
    if (!selectedNode) return null
    const base: Record<string, string | number> = {
      view: 'attempts',
      ...(timeWindow?.from ? { from: timeWindow.from } : {}),
      ...(timeWindow?.to ? { to: timeWindow.to } : {}),
      ...(failedOnly ? { status: 'failed' } : {}),
    }
    if (selectedNode.column === 'key' && selectedNode.downstreamKeyId) {
      return { ...base, downstreamKeyId: selectedNode.downstreamKeyId }
    }
    if (selectedNode.column === 'model' && selectedNode.model) {
      return { ...base, q: selectedNode.model }
    }
    if (selectedNode.column === 'upstream') {
      return {
        ...base,
        ...(selectedNode.siteId ? { siteId: selectedNode.siteId } : {}),
        ...(selectedNode.channelId
          ? { channelId: selectedNode.channelId }
          : {}),
      }
    }
    return base
  }, [selectedNode, timeWindow, failedOnly])

  const selectedChannelSearch = useMemo(() => {
    if (!selectedNode || selectedNode.column !== 'upstream') return null
    if (selectedNode.channelId && selectedNode.channelId > 0) {
      return { channelId: selectedNode.channelId }
    }
    return null
  }, [selectedNode])
  return (
    <div
      role='region'
      aria-label={t('observability.flow.actionBar.label')}
      aria-live='polite'
      className='bg-muted/40 flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3.5 py-2.5'
    >
      {selectedNode ? (
        <>
          <div className='flex min-w-0 flex-wrap items-center gap-2 text-xs'>
            <Badge variant='secondary'>
              {t(`observability.flow.columns.${selectedNode.column}`)}
            </Badge>
            <span className='text-foreground truncate font-medium'>
              {selectedNode.label}
            </span>
            {selectedNode.sublabel && (
              <span className='text-muted-foreground font-mono'>
                {selectedNode.sublabel}
              </span>
            )}
            <span className='text-muted-foreground tabular-nums'>
              {t('observability.flow.actionBar.summary', {
                requests: formatInt(selectedNode.requests, locale),
                failed: formatInt(selectedNode.failedCount, locale),
                paths: formatInt(matchingEdgeCount, locale),
              })}
            </span>
          </div>
          <div className='flex flex-wrap items-center gap-2'>
            {selectedLogsSearch && (
              <Link
                to='/proxy-logs'
                search={selectedLogsSearch}
                className={buttonVariants({
                  size: 'xs',
                  variant: 'outline',
                })}
              >
                {t('observability.flow.actionBar.goToLogs')}
                <ArrowUpRight className='size-3.5' aria-hidden='true' />
              </Link>
            )}
            {selectedChannelSearch && (
              <Link
                to='/channels'
                search={selectedChannelSearch}
                className={buttonVariants({
                  size: 'xs',
                  variant: 'outline',
                })}
              >
                {t('observability.flow.actionBar.goToChannel')}
                <ArrowUpRight className='size-3.5' aria-hidden='true' />
              </Link>
            )}
          </div>
        </>
      ) : (
        <p className='text-muted-foreground text-xs'>
          {t('observability.flow.actionBar.idleHint')}
        </p>
      )}
    </div>
  )
}

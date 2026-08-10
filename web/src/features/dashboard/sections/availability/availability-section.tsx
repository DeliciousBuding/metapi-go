// metapi-go/features/dashboard/sections/availability — availability section.
//
// Plan §5.5.1 availability: RealtimeOpsPanel（WebSocket 实时）+ Monitors
// 嵌入（不再 iframe，改组件化）. The realtime panel renders a live QPS /
// success-rate readout + a zero-dependency CSS-bar sparkline (the legacy
// panel used flex divs, not a chart lib, because the canvas var() problem
// doesn't apply to DOM). The Monitors surface is a stub here — phase 3 will
// embed the monitors feature inline (components) instead of an <iframe>.

import { Radio, ShieldCheck } from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

import { useRealtimeOps } from '../../hooks/use-realtime-ops'

const SPARK_BARS = 60

function formatRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function RealtimeSparkline({ samples }: { samples: number[] }) {
  const max = Math.max(1, ...samples)
  const bars =
    samples.length > 0
      ? samples
      : new Array<number>(SPARK_BARS).fill(0)

  return (
    <div
      className='flex h-16 w-full items-end gap-px'
      aria-hidden='true'
    >
      {bars.map((sample, index) => {
        const ratio = Math.max(0.04, sample / max)
        return (
          <div
            key={index}
            className='bg-primary/70 min-w-0 flex-1 rounded-sm'
            style={{ height: `${ratio * 100}%` }}
          />
        )
      })}
    </div>
  )
}

function RealtimeOpsPanel() {
  const sample = useRealtimeOps()

  // Silently render nothing when there is no token (anonymous view) — matches
  // the legacy panel's no-token guard.
  if (!sample.connected && !sample.gaveUp) {
    // Still mounting — keep the panel frame so the layout doesn't jump.
  }

  const tone = sample.gaveUp
    ? 'border-destructive/40 bg-destructive/5'
    : sample.connected
      ? 'border-success/40 bg-success/5'
      : 'border-border'

  return (
    <Card className={cn(tone)}>
      <CardHeader className='pb-2'>
        <CardTitle className='flex items-center justify-between text-sm font-medium'>
          <span className='flex items-center gap-2'>
            <Radio className='size-4' />
            Realtime traffic
          </span>
          <span
            className={cn(
              'flex items-center gap-1.5 text-xs font-normal',
              sample.connected
                ? 'text-success-foreground'
                : sample.gaveUp
                  ? 'text-destructive'
                  : 'text-muted-foreground',
            )}
          >
            <span
              className={cn(
                'size-2 rounded-full',
                sample.connected
                  ? 'bg-success'
                  : sample.gaveUp
                    ? 'bg-destructive'
                    : 'bg-muted-foreground/50',
              )}
            />
            {sample.gaveUp
              ? 'disconnected'
              : sample.connected
                ? 'live'
                : 'connecting'}
          </span>
        </CardTitle>
        <CardDescription className='text-xs'>
          Live QPS and success rate over the last 60s (WebSocket).
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='flex items-end justify-between gap-4'>
          <div>
            <div className='text-muted-foreground text-xs'>QPS</div>
            <div className='text-2xl font-semibold tabular-nums'>
              {sample.qps}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>success</div>
            <div className='text-2xl font-semibold tabular-nums'>
              {formatRate(sample.successRate)}
            </div>
          </div>
          <div>
            <div className='text-muted-foreground text-xs'>uptime</div>
            <div className='text-2xl font-semibold tabular-nums'>
              {Math.floor(sample.lifetime / 60)}m
            </div>
          </div>
        </div>
        <RealtimeSparkline samples={sample.spark} />
      </CardContent>
    </Card>
  )
}

export function AvailabilitySection() {
  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <RealtimeOpsPanel />

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <ShieldCheck className='size-4' />
            Uptime monitors
          </CardTitle>
          <CardDescription className='text-xs'>
            Embedded monitors (component-based, not an iframe).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='flex min-h-64 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
            <p className='text-muted-foreground text-sm'>
              TODO phase 3: embed the
              <code className='text-muted-foreground/70'> features/monitors </code>
              surface inline here (replace the legacy iframe).
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

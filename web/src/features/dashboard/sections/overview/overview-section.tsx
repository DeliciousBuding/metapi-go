// metapi-go/features/dashboard/sections/overview — overview section.
//
// Plan §5.5.1 overview: core metric cards (accounts / sites / today's checkin
// success rate / today's proxy requests) + AnnouncementBanner. The legacy
// SchedulerStatusPanel is merged in here (a compact scheduled-tasks card) per
// the plan's "砍 SchedulerStatusPanel（并入 overview）" directive.
//
// Phase 2: metric values + sparkline samples are stubbed. Phase 3 wires
// api.getDashboardSnapshot() (view=summary) for the live numbers and
// api.getSchedulerStatus() for the scheduled-tasks list.

import { Activity, CheckCircle2, Globe, Users } from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import { AnnouncementBanner } from '../../components/announcement-banner'
import { StatCard } from '../../components/stat-card'

/** Stub sparkline samples — phase 3 replaces with real 24h series. */
const STUB_SPARK_ACCOUNTS = [3, 4, 4, 5, 5, 6, 6, 7]
const STUB_SPARK_PROXY = [120, 140, 110, 160, 180, 150, 170, 190]

export function OverviewSection() {
  // TODO phase 3: const { data } = useQuery({ queryKey: ['dashboard-snapshot'],
  //   queryFn: () => api.getDashboardSnapshot() }); then unpack:
  //   activeAccounts/totalAccounts, sites, todayMetricStatus.metrics.reward.status,
  //   proxy24h.{success,total,totalTokens}, performance.{requestsPerMinute}.
  return (
    <div className='flex flex-col gap-4'>
      <AnnouncementBanner />

      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4'>
        <StatCard
          title='Accounts'
          value='—'
          hint='TODO phase 3'
          spark={STUB_SPARK_ACCOUNTS}
        />
        <StatCard
          title='Sites'
          value='—'
          hint='TODO phase 3'
        />
        <StatCard
          title="Today's checkin"
          value='—'
          hint='success rate'
        />
        <StatCard
          title='Proxy 24h'
          value='—'
          hint='requests'
          spark={STUB_SPARK_PROXY}
        />
      </div>

      {/* Merged SchedulerStatusPanel — compact scheduled-tasks card.
          Phase 3: api.getSchedulerStatus() → per-job rows (job / enabled /
          lastStatus / runs24h / success24h). */}
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <Activity className='size-4' />
            Scheduled tasks
          </CardTitle>
          <CardDescription className='text-xs'>
            Background job health (merged from the legacy SchedulerStatusPanel).
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className='flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
            <p className='text-muted-foreground text-sm'>
              TODO phase 3: render scheduler run status rows from
              <code className='text-muted-foreground/70'> api.getSchedulerStatus()</code>
            </p>
            <div className='flex items-center gap-4 text-muted-foreground'>
              <span className='flex items-center gap-1 text-xs'>
                <CheckCircle2 className='size-3' /> success
              </span>
              <span className='flex items-center gap-1 text-xs'>
                <Globe className='size-3' /> sites
              </span>
              <span className='flex items-center gap-1 text-xs'>
                <Users className='size-3' /> accounts
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Skeleton className='hidden h-1 w-full' />
    </div>
  )
}

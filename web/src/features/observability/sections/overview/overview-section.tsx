import { SlowRequestsCard } from './slow-requests-card'
import { UsageHeatmapCard } from './usage-heatmap-card'

export function OverviewSection() {
  return (
    <div className='flex flex-col gap-4'>
      <SlowRequestsCard />
      <UsageHeatmapCard />
    </div>
  )
}

import { AttentionPanel } from './attention-panel'
import { RealtimeOpsPanel } from './realtime-ops-panel'

export function AvailabilitySection() {
  return (
    <div className='grid grid-cols-1 gap-4 lg:grid-cols-2'>
      <RealtimeOpsPanel />
      <AttentionPanel />
    </div>
  )
}

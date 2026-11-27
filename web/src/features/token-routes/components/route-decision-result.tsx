import type { RouteDecision } from '../types'
import { RouteCandidateInventory } from './route-candidate-inventory'
import { RouteDecisionSummary } from './route-decision-summary'

/**
 * The one route-decision renderer. The route detail's explicit check and the
 * route workspace need the same candidate truth; rendering it twice would let
 * “unknown identity”, “not evaluated”, and cooldown recovery drift.
 */
export function RouteDecisionResult({
  decision,
  snapshotAt,
  onEditRoute,
}: {
  decision: RouteDecision
  snapshotAt?: string | null
  onEditRoute?: (id: number) => void
}) {
  return (
    <div className='space-y-2 text-xs'>
      <RouteDecisionSummary decision={decision} snapshotAt={snapshotAt} />
      <RouteCandidateInventory decision={decision} onEditRoute={onEditRoute} />
    </div>
  )
}

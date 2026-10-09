import type { UpstreamDeletionPreview } from '@/lib/api/upstream-lifecycle'

// Both the preview panel and final confirmation describe the same impact.
export function getBackupRemovalCounts(
  preview?: {
    removalImpact?: UpstreamDeletionPreview
    removals?: Record<string, number>
  } | null
): [string, number][] {
  const counts = Object.entries(
    preview?.removalImpact?.counts ?? preview?.removals ?? {}
  )
  // AxonHub source keys are deleted separately from the upstream closure;
  // impact.downstreamKeys counts keys with affected route access, not deletes.
  if (preview?.removalImpact && preview.removals?.downstream_api_keys) {
    counts.push(['downstream_api_keys', preview.removals.downstream_api_keys])
  }
  return counts.filter(
    ([section, count]) => section !== 'sourceMappings' && count > 0
  )
}

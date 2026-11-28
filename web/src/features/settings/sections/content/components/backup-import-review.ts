import type { BackupImportPreview } from './backup-import-preview-panel'
import { getBackupRemovalCounts } from './backup-import-removals'

export function getBackupImportReview(preview: BackupImportPreview) {
  const external = preview.kind === 'tables' ? null : preview.data
  const axonhub = preview.kind === 'axonhub' ? preview.data : null
  const magpie = preview.kind === 'magpie' ? preview.data : null
  const blocked = (external?.blocking?.length ?? 0) > 0
  const omitted = Object.entries(external?.notImported ?? {}).filter(
    ([, count]) => count > 0
  )
  const adapted =
    (preview.kind === 'octopus' ? (preview.data.adaptations?.length ?? 0) : 0) >
    0
  const removalImpact = external?.removalImpact
  const removals = getBackupRemovalCounts(external)
  const skipped = axonhub?.skippedChannels
    ? axonhub.skippedChannels.map((channel) => ({
        ...channel,
        reasonPrefix: 'axonhubReasons' as const,
      }))
    : (magpie?.skippedProviders ?? []).map((provider) => ({
        ...provider,
        reasonPrefix: 'magpieReasons' as const,
      }))
  const skippedGroups = (magpie?.skippedGroups ?? []).map((group) => ({
    ...group,
    reasonPrefix: 'magpieReasons' as const,
  }))
  const residuals = axonhub?.residuals ?? magpie?.residuals ?? []
  const unsupportedProtocols = residuals.flatMap((residual) => {
    const match = /^declared_protocol_not_servable:([^\s]+)/.exec(residual)
    return match ? [match[1]] : []
  })
  const otherResiduals = residuals.filter(
    (residual) => !residual.startsWith('declared_protocol_not_servable:')
  )
  const routable = Object.entries((axonhub ?? magpie)?.routable ?? {}).filter(
    ([, count]) => count > 0
  )
  // Only the Octopus importer needs an explicit "channels only" confirmation.
  // AxonHub and Magpie imports commit their compiled graph and list every
  // residual for review rather than pretending the snapshot was lossless.
  const needsAcknowledgement =
    preview.kind === 'octopus' && (omitted.length > 0 || adapted)
  const hasExternalReview =
    skipped.length > 0 ||
    skippedGroups.length > 0 ||
    residuals.length > 0 ||
    omitted.length > 0 ||
    adapted
  let status = 'ready'
  let variant: 'success' | 'warning' | 'destructive' = 'success'
  if (blocked) {
    status = 'blocked'
    variant = 'destructive'
  } else if (hasExternalReview || removals.length > 0) {
    status = 'review'
    variant = 'warning'
  }
  const counts = external
    ? Object.entries(external.sections).filter(([, count]) => count > 0)
    : []
  return {
    external,
    axonhub,
    magpie,
    blocked,
    omitted,
    adapted,
    removalImpact,
    removals,
    skipped,
    skippedGroups,
    residuals,
    unsupportedProtocols,
    otherResiduals,
    routable,
    needsAcknowledgement,
    hasExternalReview,
    status,
    variant,
    counts,
  }
}
export type BackupImportReview = ReturnType<typeof getBackupImportReview>

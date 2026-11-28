import { api } from '@/lib/api'
import {
  detectExternalBackupSourceText,
  stripBackupBOM,
} from '@/lib/helpers/external-backup-source'

import type {
  BackupImportTablePlan,
  AxonHubV14Preview,
  MagpieBackupPreview,
  OctopusV5Preview,
} from './backup-import-preview-panel'

function isExternalPreview(
  plan: BackupImportTablePlan | OctopusV5Preview | AxonHubV14Preview
): plan is OctopusV5Preview | AxonHubV14Preview {
  const candidate = plan as OctopusV5Preview
  const impact = candidate.removalImpact
  if (
    impact &&
    (impact.kind !== 'source' ||
      impact.id !== 0 ||
      typeof impact.revision !== 'string' ||
      !impact.revision.trim() ||
      typeof impact.counts !== 'object' ||
      impact.counts === null ||
      Array.isArray(impact.counts))
  ) {
    return false
  }
  return (
    typeof candidate.source === 'string' &&
    typeof candidate.originKey === 'string' &&
    typeof candidate.sections === 'object' &&
    candidate.sections !== null &&
    !Array.isArray(candidate.sections) &&
    (candidate.blocking === undefined || Array.isArray(candidate.blocking))
  )
}

export async function fetchBackupImportPreview(input: {
  raw: string
  originKey?: string
  magpiePassphrase?: string
}) {
  const { raw, originKey, magpiePassphrase } = input
  const data = JSON.parse(stripBackupBOM(raw)) as unknown
  const external = detectExternalBackupSourceText(raw)
  const result = (await api.previewBackupImport(data, {
    externalOriginKey: external ? originKey : undefined,
    magpiePassphrase,
  })) as {
    success?: boolean
    plan?:
      | BackupImportTablePlan
      | OctopusV5Preview
      | AxonHubV14Preview
      | MagpieBackupPreview
  }
  if (external) {
    const plan = result.plan
    if (!plan || !isExternalPreview(plan)) {
      throw new Error('Invalid external import preview')
    }
    if (plan.originKey !== originKey) {
      throw new Error('Preview origin key does not match the request')
    }
    if (external === 'axonhub-v1.4') {
      const axonhub = plan as AxonHubV14Preview
      if (
        typeof axonhub.routable !== 'object' ||
        axonhub.routable === null ||
        Array.isArray(axonhub.routable) ||
        (axonhub.residuals !== undefined &&
          !Array.isArray(axonhub.residuals)) ||
        (axonhub.skippedChannels !== undefined &&
          !Array.isArray(axonhub.skippedChannels))
      ) {
        throw new Error('Invalid AxonHub v1.4 import preview')
      }
      return {
        kind: 'axonhub' as const,
        data: axonhub,
      }
    }
    if (external === 'magpie-backup') {
      const magpie = plan as MagpieBackupPreview
      if (
        typeof magpie.routable !== 'object' ||
        magpie.routable === null ||
        Array.isArray(magpie.routable) ||
        (magpie.residuals !== undefined && !Array.isArray(magpie.residuals)) ||
        (magpie.skippedProviders !== undefined &&
          !Array.isArray(magpie.skippedProviders)) ||
        (magpie.skippedGroups !== undefined &&
          !Array.isArray(magpie.skippedGroups))
      ) {
        throw new Error('Invalid Magpie backup import preview')
      }
      return {
        kind: 'magpie' as const,
        data: magpie,
      }
    }
    const octopus = plan as OctopusV5Preview
    if (
      octopus.adaptations !== undefined &&
      !Array.isArray(octopus.adaptations)
    ) {
      throw new Error('Invalid Octopus v5 import preview')
    }
    return {
      kind: 'octopus' as const,
      data: octopus,
    }
  }
  if (
    !result.plan ||
    typeof result.plan !== 'object' ||
    Array.isArray(result.plan)
  ) {
    throw new Error('Invalid backup import preview')
  }
  return {
    kind: 'tables' as const,
    tables: result.plan as BackupImportTablePlan,
  }
}

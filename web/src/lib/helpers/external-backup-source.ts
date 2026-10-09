/**
 * External (non-Metapi) backup envelopes the import endpoint accepts.
 *
 * Detection is deliberately envelope-only and duplicated in the backend
 * (`service/backup/external_source.go` + `IsAxonHubV14Payload`): the client
 * decides which headers to send, the server decides what to parse. A payload
 * that only partly looks like one of these sources is still handed to the
 * importer so the server owns the rejection.
 */
export type ExternalBackupSource = 'octopus-v5' | 'axonhub-v1.4'

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function detectExternalBackupSource(
  value: unknown
): ExternalBackupSource | null {
  if (!isRecord(value)) return null
  if (value.version === 5 && typeof value.exported_at === 'string') {
    return 'octopus-v5'
  }
  if (
    value.version === '1.4' &&
    typeof value.timestamp === 'string' &&
    'channels' in value &&
    'models' in value
  ) {
    return 'axonhub-v1.4'
  }
  return null
}

export function detectExternalBackupSourceText(
  raw: string
): ExternalBackupSource | null {
  try {
    return detectExternalBackupSource(JSON.parse(raw) as unknown)
  } catch {
    return null
  }
}

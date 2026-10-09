import { describe, expect, it } from 'vitest'

import {
  detectExternalBackupSource,
  detectExternalBackupSourceText,
} from '../external-backup-source'

describe('detectExternalBackupSource', () => {
  it('recognizes an Octopus v5 envelope', () => {
    expect(
      detectExternalBackupSource({ version: 5, exported_at: '2026-01-01' })
    ).toBe('octopus-v5')
  })

  it('recognizes an AxonHub v1.4 envelope', () => {
    expect(
      detectExternalBackupSource({
        version: '1.4',
        timestamp: '2026-01-01T00:00:00Z',
        channels: [],
        models: [],
      })
    ).toBe('axonhub-v1.4')
  })

  it('does not claim a Metapi tables payload', () => {
    expect(
      detectExternalBackupSource({
        tables: { sites: [] },
        metadata: { version: '2.1' },
      })
    ).toBeNull()
  })

  it('requires the AxonHub configuration sections, not just a version', () => {
    expect(
      detectExternalBackupSource({ version: '1.4', timestamp: 'now' })
    ).toBeNull()
  })

  it('rejects non-objects and malformed text', () => {
    expect(detectExternalBackupSource(null)).toBeNull()
    expect(detectExternalBackupSource([])).toBeNull()
    expect(detectExternalBackupSourceText('not json')).toBeNull()
    expect(
      detectExternalBackupSourceText(
        JSON.stringify({
          version: '1.4',
          timestamp: 'now',
          channels: [],
          models: [],
        })
      )
    ).toBe('axonhub-v1.4')
  })
})

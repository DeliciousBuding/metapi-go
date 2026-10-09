import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { BackupImportPreviewPanel } from '../backup-import-preview-panel'

afterEach(cleanup)

describe('backup import preview eligibility', () => {
  it('lists removals with the origin isolation and final confirmation notice', () => {
    render(
      <BackupImportPreviewPanel
        preview={{
          kind: 'octopus',
          data: {
            source: 'octopus-v5',
            originKey: 'fixture',
            sections: { channels: 1 },
            removals: { channelGrants: 2, channels: 1 },
          },
        }}
        acknowledged={false}
        onAcknowledge={vi.fn()}
      />
    )
    expect(
      screen.getByText(
        i18n.t('settings.content.importExport.octopusReplacementTitle')
      )
    ).toBeVisible()
    expect(
      screen.getByText(
        i18n.t('settings.content.importExport.octopusReplacementDescription')
      )
    ).toBeVisible()
    expect(screen.getByText(/· 2/)).toBeVisible()
  })
  it('never promises all sections are supported when configuration blocks import', () => {
    render(
      <BackupImportPreviewPanel
        preview={{
          kind: 'octopus',
          data: {
            source: 'octopus-v5',
            originKey: 'fixture',
            sections: { channels: 1 },
            blocking: ['customPaths'],
          },
        }}
        acknowledged={false}
        onAcknowledge={vi.fn()}
      />
    )
    expect(screen.getByText(/customPaths/)).toBeInTheDocument()
    expect(
      screen.queryByText(
        i18n.t('settings.content.importExport.octopusAllSectionsSupported')
      )
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('checkbox')).not.toBeInTheDocument()
  })
})

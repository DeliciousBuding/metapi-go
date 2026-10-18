import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { BackupImportPreviewPanel } from '../backup-import-preview-panel'

afterEach(async () => {
  cleanup()
  await i18n.changeLanguage('en')
})

describe('backup import preview eligibility', () => {
  it('keeps source counts separate from the executable AxonHub plan', async () => {
    render(
      <BackupImportPreviewPanel
        preview={{
          kind: 'axonhub',
          data: {
            source: 'axonhub-v1.4',
            version: '1.4',
            originKey: 'fixture',
            sections: { channels: 7, projects: 3 },
            routable: { channels: 2 },
            residuals: ['channel-4:manual_models_not_imported_until_synced'],
          },
        }}
        acknowledged={false}
        onAcknowledge={vi.fn()}
      />
    )
    expect(screen.getByText('2')).toBeVisible()
    expect(screen.queryByText('7')).not.toBeInTheDocument()
    expect(screen.getByText(/Source channel #4:/)).toBeVisible()
    const toggle = screen.getByRole('button', {
      name: 'Source record counts',
    })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(toggle)
    expect(await screen.findByText('7')).toBeVisible()
    expect(screen.getByText('3')).toBeVisible()
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(
      within(screen.getByRole('region', { name: 'Import plan' })).getByText('2')
    ).toBeVisible()
  })
  it.each(['zh-CN', 'en'])(
    'explains AxonHub differences in %s and preserves source identifiers',
    async (language) => {
      await i18n.changeLanguage(language === 'zh-CN' ? 'zhCN' : language)
      const residuals = [
        'channel-4:manual_models_not_imported_until_synced',
        'project_active_profiles_flattened_into_key_channel_boundaries',
        'model_has_no_importable_channel:vendor:model-a',
        'model_strategy_approximated:adaptive:vendor:model-b',
        'channel-7:future_behavior:detail',
      ]
      render(
        <BackupImportPreviewPanel
          preview={{
            kind: 'axonhub',
            data: {
              source: 'axonhub-v1.4',
              version: '1.4',
              originKey: 'fixture',
              sections: { channels: 1 },
              routable: { channels: 1 },
              residuals,
            },
          }}
          acknowledged={false}
          onAcknowledge={vi.fn()}
        />
      )
      expect(screen.queryByText(residuals[0])).not.toBeInTheDocument()
      expect(screen.queryByText(residuals[1])).not.toBeInTheDocument()
      expect(
        screen.getByText(
          language === 'zh-CN'
            ? '来源渠道 #4：手动添加的模型未迁移，请在来源端同步后重新导入。'
            : 'Source channel #4: Manually added models were not migrated. Sync them in the source before importing again.'
        )
      ).toBeVisible()
      expect(screen.getByText(/vendor:model-a/)).toBeVisible()
      expect(screen.getByText(/vendor:model-b/)).toHaveTextContent('adaptive')
      expect(screen.getByText(residuals[4])).toBeVisible()
    }
  )
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

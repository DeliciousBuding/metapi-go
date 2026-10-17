import type { TFunction } from 'i18next'

export function formatAxonHubResidual(residual: string, t: TFunction): string {
  const channel = /^channel-(\d+):(.+)$/.exec(residual)
  const value = channel?.[2] ?? residual
  const [reason, ...parts] = value.split(':')
  const message = t(
    `settings.content.importExport.axonhubResidualReasons.${reason}`,
    {
      defaultValue: value,
      detail: parts.join(':'),
      strategy: parts[0],
      model: parts.slice(1).join(':'),
    }
  )
  if (message === value) return residual
  return channel
    ? t('settings.content.importExport.axonhubResidualChannel', {
        id: channel[1],
        message,
      })
    : message
}

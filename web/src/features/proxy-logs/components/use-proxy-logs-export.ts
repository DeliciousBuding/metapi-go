import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { loadProxyLogsForCsv, proxyLogsToCsv } from '../lib/proxy-logs-csv'
import type { ProxyLogsQuery } from '../types'

export function useProxyLogsExport(queryPayload: ProxyLogsQuery) {
  const { t } = useTranslation()
  const [isExporting, setIsExporting] = useState(false)

  async function handleExportCsv() {
    setIsExporting(true)
    try {
      const { rows, truncated } = await loadProxyLogsForCsv(
        queryPayload,
        (params) => api.getProxyLogsQuery(params)
      )
      const csv = proxyLogsToCsv(rows, t)
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `metapi-proxy-logs-${formatExportStamp()}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
      if (truncated) {
        toast.warning(t('proxyLogs.page.exportCsvTruncated'), {
          description: t('proxyLogs.page.exportCsvTruncatedHint', {
            count: rows.length,
          }),
        })
      } else {
        toast.success(
          t('proxyLogs.page.exportCsvToast', { count: rows.length })
        )
      }
    } catch (exportError) {
      toast.error(
        t('proxyLogs.page.exportCsvFailed', {
          message:
            exportError instanceof Error ? exportError.message : 'unknown',
        })
      )
    } finally {
      setIsExporting(false)
    }
  }

  return { isExporting, handleExportCsv }
}

function formatExportStamp(): string {
  const now = new Date()
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
}

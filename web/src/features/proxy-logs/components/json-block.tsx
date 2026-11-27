import { Check as CheckIcon, Copy as CopyIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'

function prettyPrintJson(value: unknown): string {
  if (typeof value === 'string') {
    const trimmed = value.trim()
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2)
    } catch {
      return trimmed
    }
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export function JsonBlock({ value }: { value: unknown }) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)
  const text = useMemo(() => prettyPrintJson(value), [value])
  async function handleCopy() {
    if (await copyText(text)) {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } else {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('common.copyFailed'))
    }
  }
  return (
    <div className='relative'>
      <Button
        variant='ghost'
        size='icon-sm'
        onClick={handleCopy}
        className='absolute top-1 right-1'
        aria-label={t('proxyLogs.detail.copy')}
      >
        {copied ? (
          <CheckIcon className='size-3.5' />
        ) : (
          <CopyIcon className='size-3.5' />
        )}
      </Button>
      <pre className='bg-muted max-h-80 overflow-auto rounded p-2 pr-9 text-xs leading-relaxed break-all whitespace-pre-wrap'>
        {text}
      </pre>
    </div>
  )
}

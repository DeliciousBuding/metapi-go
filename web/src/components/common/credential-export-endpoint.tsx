import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

export function CredentialExportEndpoint({
  clientBase,
  apiKey,
  keyMasked,
  showKey,
  copiedKey,
  onToggleKey,
  onCopyKey,
  onCopyEndpoint,
}: {
  clientBase: string
  apiKey: string
  keyMasked?: string
  showKey: boolean
  copiedKey: boolean
  onToggleKey: () => void
  onCopyKey: () => void
  onCopyEndpoint: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='bg-muted/40 rounded-lg border p-3'>
      <p className='text-muted-foreground mb-1 text-xs font-medium'>
        {t('connect.endpointLabel')}
      </p>
      <div className='flex items-center gap-2'>
        <code className='text-foreground flex-1 truncate font-mono text-sm'>
          {clientBase}
        </code>
        <Button
          variant='outline'
          size='icon-sm'
          aria-label={t('connect.copyEndpoint')}
          onClick={onCopyEndpoint}
        >
          <Copy className='size-3.5' />
        </Button>
      </div>
      <div className='mt-2 flex items-center gap-2 border-t pt-2'>
        <code className='text-muted-foreground flex-1 truncate font-mono text-xs'>
          {showKey ? apiKey : (keyMasked ?? 'sk-…')}
        </code>
        <Button
          variant='outline'
          size='icon-sm'
          aria-label={t(showKey ? 'connect.hideKey' : 'connect.showKey')}
          onClick={onToggleKey}
        >
          {showKey ? (
            <EyeOff className='size-3.5' />
          ) : (
            <Eye className='size-3.5' />
          )}
        </Button>
        <Button
          variant='outline'
          size='icon-sm'
          aria-label={t(
            copiedKey ? 'connect.toast.keyCopied' : 'connect.copyKey'
          )}
          onClick={onCopyKey}
        >
          {copiedKey ? (
            <Check className='text-success size-3.5' />
          ) : (
            <Copy className='size-3.5' />
          )}
        </Button>
      </div>
    </div>
  )
}

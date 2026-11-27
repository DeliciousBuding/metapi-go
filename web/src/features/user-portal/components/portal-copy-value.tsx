import { Copy } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { copyText } from '@/lib/clipboard'
import { toast } from '@/lib/toast'

export function CopyValue({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation()
  return (
    <Button
      type='button'
      variant='outline'
      size='sm'
      onClick={async () => {
        if (await copyText(value)) toast.success(t('userPortal.copied'))
        else toast.error(t('userPortal.copyFailed'))
      }}
    >
      <Copy aria-hidden='true' />
      {label}
    </Button>
  )
}

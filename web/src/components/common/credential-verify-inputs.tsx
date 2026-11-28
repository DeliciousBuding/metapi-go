import { Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { CredentialVerifyProtocol as Protocol } from './credential-verify-protocol'

export function CredentialVerifyInputs({
  models,
  selectedModel,
  protocol,
  protocolLabelId,
  busy,
  verifying,
  onModelChange,
  onProtocolChange,
  onVerify,
}: {
  models: string[]
  selectedModel: string
  protocol: Protocol
  protocolLabelId: string
  busy: boolean
  verifying: boolean
  onModelChange: (model: string) => void
  onProtocolChange: (protocol: Protocol) => void
  onVerify: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-2'>
      <div className='space-y-1'>
        <p
          id={protocolLabelId}
          className='text-muted-foreground text-xs font-medium'
        >
          {t('connect.verify.protocol.label')}
        </p>
        <RadioGroup
          className='flex flex-wrap gap-x-4 gap-y-2'
          value={protocol}
          aria-labelledby={protocolLabelId}
          onValueChange={(value) => {
            if (!value || value === protocol) return
            onProtocolChange(value as Protocol)
          }}
        >
          <div className='flex items-center gap-1.5'>
            <RadioGroupItem id={`${protocolLabelId}-chat`} value='chat' />
            <Label htmlFor={`${protocolLabelId}-chat`}>
              {t('connect.verify.protocol.chat')}
            </Label>
          </div>
          <div className='flex items-center gap-1.5'>
            <RadioGroupItem
              id={`${protocolLabelId}-responses`}
              value='responses'
            />
            <Label htmlFor={`${protocolLabelId}-responses`}>
              {t('connect.verify.protocol.responses')}
            </Label>
          </div>
          <div className='flex items-center gap-1.5'>
            <RadioGroupItem
              id={`${protocolLabelId}-messages`}
              value='messages'
            />
            <Label htmlFor={`${protocolLabelId}-messages`}>
              {t('connect.verify.protocol.messages')}
            </Label>
          </div>
        </RadioGroup>
      </div>
      <div className='flex flex-wrap items-center gap-2'>
        <Select
          items={models.map((model) => ({
            value: model,
            label: model,
          }))}
          value={selectedModel}
          onValueChange={(value) => {
            onModelChange(value ?? '')
          }}
        >
          <SelectTrigger
            size='sm'
            aria-label={t('connect.verify.modelAria')}
            className='w-56'
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {models.map((model) => (
              <SelectItem key={model} value={model}>
                {model}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button size='sm' disabled={!selectedModel || busy} onClick={onVerify}>
          {verifying ? <Loader2 className='size-3.5 animate-spin' /> : null}
          {t('connect.verify.testCta')}
        </Button>
      </div>
    </div>
  )
}

// Checks model access and one non-streaming request in the protocol the user
// explicitly selects. The request consumes upstream quota; the result links to
// this key's request logs and never auto-retries through another protocol.

import { CheckCircle2, Loader2, XCircle } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { CredentialVerifyInputs } from './credential-verify-inputs'
import type { CredentialVerifyProtocol as Protocol } from './credential-verify-protocol'
import {
  CredentialVerifyResult,
  CredentialLogsLink,
} from './credential-verify-result'
import {
  type CredentialVerificationInput,
  useCredentialVerification,
} from './use-credential-verification'

export type { CredentialVerifyProtocol } from './credential-verify-protocol'

export function CredentialVerifySection(
  props: CredentialVerificationInput & {
    onProtocolChange: (protocol: Protocol) => void
  }
) {
  const {
    keyId,
    apiKey,
    model: selectedModel,
    protocol,
    onModelChange,
    onProtocolChange,
  } = props
  const { t } = useTranslation()
  const {
    modelsState,
    verificationState,
    busy,
    runModelsCheck,
    runVerificationCheck,
    resetVerification,
  } = useCredentialVerification(props)
  const protocolLabelId = useId()
  return (
    <div className='space-y-2 rounded-lg border p-3'>
      <p className='text-muted-foreground text-xs font-medium'>
        {t('connect.verify.title')}
      </p>
      <p className='text-muted-foreground text-xs'>
        {t('connect.verify.description')}
      </p>

      <div className='flex flex-wrap items-center gap-2'>
        <Button
          size='sm'
          variant='outline'
          disabled={!apiKey || busy}
          onClick={() => void runModelsCheck()}
        >
          {modelsState.phase === 'loading' ? (
            <Loader2 className='size-3.5 animate-spin' />
          ) : null}
          {t('connect.verify.modelsCta')}
        </Button>
        {modelsState.phase === 'ok' ? (
          <span className='text-muted-foreground flex items-center gap-1 text-xs'>
            <CheckCircle2 className='text-success-soft-fg size-3.5' />
            {t('connect.verify.modelsOk', { count: modelsState.models.length })}
          </span>
        ) : null}
      </div>

      {modelsState.phase === 'ok' ? (
        <div className='space-y-2 border-t pt-2'>
          {modelsState.models.length === 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('connect.verify.noModels')}
            </p>
          ) : (
            <CredentialVerifyInputs
              models={modelsState.models}
              selectedModel={selectedModel}
              protocol={protocol}
              protocolLabelId={protocolLabelId}
              busy={busy}
              verifying={verificationState.phase === 'loading'}
              onVerify={() => void runVerificationCheck()}
              onModelChange={(model) => {
                onModelChange(model)
                resetVerification()
              }}
              onProtocolChange={(value) => {
                onProtocolChange(value)
                resetVerification()
              }}
            />
          )}

          <CredentialVerifyResult
            verificationState={verificationState}
            keyId={keyId}
          />
        </div>
      ) : null}

      {modelsState.phase === 'fail' ? (
        <div className='space-y-2 border-t pt-2'>
          <p className='text-destructive-soft-fg flex items-start gap-1 text-xs break-all'>
            <XCircle className='mt-0.5 size-3.5 shrink-0' />
            <span>{modelsState.message}</span>
          </p>
          <CredentialLogsLink keyId={keyId} />
        </div>
      ) : null}
    </div>
  )
}

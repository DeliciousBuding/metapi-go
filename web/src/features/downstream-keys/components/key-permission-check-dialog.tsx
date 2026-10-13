// metapi-go/features/downstream-keys — key permission check dialog.
//
// The "can this key call this model on this endpoint right now" answer,
// computed server-side and read-only: the admission chain is evaluated
// without consuming quota, incrementing rate windows, leasing credentials, or
// calling upstreams, and the route explanation runs under the key's policy.
// The dialog renders the server's verdict without re-deriving any of it
// client-side — the blockers and window numbers are the answer, not a hint.

import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api, type KeyContextDecisionResponse } from '@/lib/api'
import { toast } from '@/lib/toast'

// Same endpoint surface as the route decision panel.
const endpoints = [
  ['/v1/chat/completions', 'OpenAI Chat'],
  ['/v1/responses', 'OpenAI Responses'],
  ['/v1/messages', 'Anthropic Messages'],
  ['/v1beta/models/{model}:generateContent', 'Gemini generateContent'],
  ['/v1/messages/count_tokens', 'Anthropic count_tokens'],
  ['/v1/embeddings', 'OpenAI Embeddings'],
] as const

export function KeyPermissionCheckDialog(props: {
  keyId: number | null
  keyName: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const [model, setModel] = useState('')
  const [endpoint, setEndpoint] = useState<string>(endpoints[0][0])
  const [clientIp, setClientIp] = useState('')

  const checkMutation = useMutation({
    mutationFn: async () => {
      if (!props.keyId) throw new Error('no key selected')
      return api.getRouteDecisionKeyContext({
        keyId: props.keyId,
        model: model.trim(),
        endpoint,
        clientIp: clientIp.trim() || undefined,
      })
    },
    onError: (error) => {
      toast.error(
        t('settings.downstream.keys.check.loadFailed', {
          message: error instanceof Error ? error.message : String(error),
        })
      )
    },
  })
  const result: KeyContextDecisionResponse | undefined = checkMutation.data

  const run = () => {
    if (!model.trim()) return
    checkMutation.mutate()
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='sm:max-w-xl'>
        <DialogHeader>
          <DialogTitle>
            {t('settings.downstream.keys.check.title', { name: props.keyName })}
          </DialogTitle>
          <DialogDescription>
            {t('settings.downstream.keys.check.description')}
          </DialogDescription>
        </DialogHeader>

        <div className='grid gap-3'>
          <div className='grid gap-1.5'>
            <Label htmlFor='key-check-model'>
              {t('settings.downstream.keys.check.modelLabel')}
            </Label>
            <Input
              id='key-check-model'
              value={model}
              onChange={(event) => setModel(event.target.value)}
              placeholder='gpt-4o'
            />
          </div>
          <div className='grid gap-1.5'>
            <Label>{t('settings.downstream.keys.check.endpointLabel')}</Label>
            <Select
              value={endpoint}
              onValueChange={(value) => setEndpoint(value ?? endpoints[0][0])}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {endpoints.map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='key-check-ip'>
              {t('settings.downstream.keys.check.clientIpLabel')}
            </Label>
            <Input
              id='key-check-ip'
              value={clientIp}
              onChange={(event) => setClientIp(event.target.value)}
              placeholder='203.0.113.10'
            />
          </div>
        </div>

        {result ? <CheckResult result={result} /> : null}

        <DialogFooter>
          <Button
            onClick={run}
            disabled={!model.trim() || checkMutation.isPending || !props.keyId}
          >
            {checkMutation.isPending
              ? t('settings.downstream.keys.check.running')
              : t('settings.downstream.keys.check.run')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CheckResult(props: { result: KeyContextDecisionResponse }) {
  const { t } = useTranslation()
  const { result } = props
  const admission = result.keyContext.admission
  const candidates = result.decision.candidates ?? []
  const eligible = candidates.filter(
    (candidate) => candidate.eligibility === 'eligible'
  ).length
  const blocked = candidates.filter(
    (candidate) => candidate.eligibility === 'blocked'
  ).length

  return (
    <div className='flex flex-col gap-3 border-t pt-3'>
      <div className='flex items-center gap-2'>
        {admission.verdict === 'allow' ? (
          <Badge variant='default'>
            {t('settings.downstream.keys.check.allow')}
          </Badge>
        ) : (
          <Badge variant='destructive'>
            {t('settings.downstream.keys.check.deny')}
          </Badge>
        )}
        <span className='text-muted-foreground text-xs'>
          {t('settings.downstream.keys.check.readOnlyNote')}
        </span>
      </div>

      {admission.blockers.length > 0 ? (
        <ul className='flex flex-col gap-1.5'>
          {admission.blockers.map((blocker) => (
            <li
              key={`${blocker.dimension}-${blocker.code}`}
              className='flex items-start gap-2 text-sm'
            >
              <Badge variant='outline' className='shrink-0'>
                {t(
                  `settings.downstream.keys.check.dimensions.${blocker.dimension}`
                )}
              </Badge>
              <span className='min-w-0'>{blocker.message}</span>
            </li>
          ))}
        </ul>
      ) : null}

      <AdmissionWindows result={result} />

      <div className='text-muted-foreground text-xs'>
        {result.decision.matched
          ? t('settings.downstream.keys.check.decisionMatched', {
              pattern:
                result.decision.matchedRoutePattern ??
                result.decision.modelPattern ??
                '',
              eligible,
              blocked,
            })
          : t('settings.downstream.keys.check.decisionUnmatched')}
      </div>
    </div>
  )
}

function AdmissionWindows(props: { result: KeyContextDecisionResponse }) {
  const { t } = useTranslation()
  const admission = props.result.keyContext.admission
  const rows: Array<{ label: string; value: string }> = []
  const push = (
    label: string,
    used: number,
    max: number | null | undefined
  ) => {
    rows.push({
      label,
      value: max == null ? String(used) : `${used} / ${max}`,
    })
  }
  if (admission.cost) {
    push(
      t('settings.downstream.keys.check.cost'),
      admission.cost.used,
      admission.cost.max
    )
  }
  if (admission.requests) {
    push(
      t('settings.downstream.keys.check.requests'),
      admission.requests.used,
      admission.requests.max
    )
  }
  if (admission.rate) {
    push(
      t('settings.downstream.keys.check.rpm'),
      admission.rate.rpm.used,
      admission.rate.rpm.max
    )
    push(
      t('settings.downstream.keys.check.tpm'),
      admission.rate.tpm.used,
      admission.rate.tpm.max
    )
  }
  if (admission.quota) {
    if (admission.quota.requestsMax != null) {
      push(
        t('settings.downstream.keys.check.quotaRequests'),
        admission.quota.requestsUsed,
        admission.quota.requestsMax
      )
    }
    if (admission.quota.tokensMax != null) {
      push(
        t('settings.downstream.keys.check.quotaTokens'),
        admission.quota.tokensUsed,
        admission.quota.tokensMax
      )
    }
    if (admission.quota.costMax != null) {
      push(
        t('settings.downstream.keys.check.quotaCost'),
        admission.quota.costUsed,
        admission.quota.costMax
      )
    }
  }
  if (rows.length === 0) return null
  return (
    <div className='flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums'>
      {rows.map((row) => (
        <span key={row.label}>
          <span className='text-muted-foreground'>{row.label}</span> {row.value}
        </span>
      ))}
      {admission.rate ? (
        <span className='text-muted-foreground'>
          {t('settings.downstream.keys.check.rateScopeNote')}
        </span>
      ) : null}
    </div>
  )
}

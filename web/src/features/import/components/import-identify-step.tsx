import { Check as CheckIcon, TriangleAlert as AlertIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Spinner } from '@/components/ui/spinner'

import { canonicalizeUrl } from '../lib/utils'
import type { ImportDuplicateStrategy } from '../types'
import type { CandidateStepProps } from './import-candidate'
import type { BindImportField } from './use-import-fields'

export function ImportIdentifyStep({
  candidates,
  updateCandidate,
  existingUrls,
  bindField,
}: CandidateStepProps & {
  existingUrls: ReadonlySet<string>
  bindField: BindImportField
}) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-3'>
      {candidates.map((candidate) => {
        const duplicate = existingUrls.has(canonicalizeUrl(candidate.url))
        return (
          <div key={candidate.id} className='grid gap-3 rounded-lg border p-3'>
            <div className='grid gap-2'>
              <Label htmlFor={`${candidate.id}-name`}>
                {t('import.identify.name')}
              </Label>
              <Input
                id={`${candidate.id}-name`}
                value={candidate.name}
                onChange={(event) =>
                  updateCandidate(candidate.id, {
                    name: event.target.value,
                  })
                }
              />
            </div>
            <div className='text-muted-foreground truncate font-mono text-xs'>
              {candidate.url}
            </div>
            <div className='grid gap-2'>
              <Label htmlFor={`${candidate.id}-platform`}>
                {t('import.identify.platform')}
              </Label>
              <div className='flex items-center gap-2'>
                <Input
                  id={`${candidate.id}-platform`}
                  {...bindField(`${candidate.id}-platform`, (platform) =>
                    updateCandidate(candidate.id, {
                      platform,
                      detected: false,
                      confidence: null,
                    })
                  )}
                  value={candidate.platform}
                  className='flex-1'
                />
                {candidate.detecting && <Spinner />}
                {!candidate.detecting && candidate.detected && (
                  <Badge variant='secondary'>
                    <CheckIcon className='size-3' />
                    {t('import.identify.detected', {
                      confidence: Math.round((candidate.confidence ?? 0) * 100),
                    })}
                  </Badge>
                )}
              </div>
            </div>

            {duplicate && (
              <Notice tone='warning'>
                <AlertIcon className='text-warning mt-0.5 size-4 shrink-0' />
                <div className='min-w-0 flex-1 space-y-2'>
                  <p>{t('import.identify.duplicateWarning')}</p>
                  <RadioGroup
                    value={candidate.duplicateStrategy}
                    onValueChange={(value) =>
                      updateCandidate(candidate.id, {
                        duplicateStrategy: value as ImportDuplicateStrategy,
                      })
                    }
                  >
                    <div className='flex items-center gap-2'>
                      <RadioGroupItem
                        id={`${candidate.id}-skip`}
                        value='skip'
                      />
                      <Label htmlFor={`${candidate.id}-skip`}>
                        {t('import.identify.skip')}
                      </Label>
                    </div>
                    <div className='flex items-center gap-2'>
                      <RadioGroupItem
                        id={`${candidate.id}-merge`}
                        value='merge'
                      />
                      <Label htmlFor={`${candidate.id}-merge`}>
                        {t('import.identify.merge')}
                      </Label>
                    </div>
                  </RadioGroup>
                </div>
              </Notice>
            )}
          </div>
        )
      })}
    </div>
  )
}

import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import type { CandidateStepProps } from './import-candidate'
import type { BindImportField } from './use-import-fields'

export function ImportRoutesStep({
  candidates,
  updateCandidate,
  bindField,
}: CandidateStepProps & { bindField: BindImportField }) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-3'>
      <p className='text-muted-foreground text-xs'>{t('import.routes.hint')}</p>
      {candidates.map((candidate) => (
        <div
          key={candidate.id}
          className='flex items-center justify-between gap-3 rounded-lg border p-3'
        >
          <div className='min-w-0'>
            <div className='truncate text-sm font-medium'>{candidate.name}</div>
            <div className='text-muted-foreground truncate text-xs'>
              {candidate.platform}
            </div>
          </div>
          <div className='flex items-center gap-2'>
            <Label htmlFor={`${candidate.id}-weight`}>
              {t('import.routes.weight')}
            </Label>
            <Input
              id={`${candidate.id}-weight`}
              {...bindField(`${candidate.id}-weight`, (value) =>
                updateCandidate(candidate.id, { weight: Number(value) })
              )}
              type='number'
              min={0}
              step={0.1}
              value={candidate.weight}
              className='w-24'
            />
          </div>
        </div>
      ))}
    </div>
  )
}

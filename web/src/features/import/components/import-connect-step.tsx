import { useTranslation } from 'react-i18next'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import type { CandidateStepProps } from './import-candidate'

export function ImportConnectStep({
  candidates,
  updateCandidate,
}: CandidateStepProps) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-3'>
      {candidates.map((candidate) => (
        <div key={candidate.id} className='grid gap-3 rounded-lg border p-3'>
          <div className='flex items-center justify-between gap-3'>
            <div className='min-w-0'>
              <div className='truncate text-sm font-medium'>
                {candidate.name}
              </div>
              <div className='text-muted-foreground truncate text-xs'>
                {candidate.url}
              </div>
            </div>
            <Switch
              checked={candidate.includeAccount}
              onCheckedChange={(checked) =>
                updateCandidate(candidate.id, {
                  includeAccount: checked,
                })
              }
              aria-label={t('import.connect.includeAccount', {
                name: candidate.name,
              })}
            />
          </div>
          {candidate.includeAccount && (
            <div className='grid gap-2 sm:grid-cols-3'>
              <div className='grid gap-1.5'>
                <Label htmlFor={`${candidate.id}-username`}>
                  {t('import.connect.username')}
                </Label>
                <Input
                  id={`${candidate.id}-username`}
                  value={candidate.username}
                  onChange={(event) =>
                    updateCandidate(candidate.id, {
                      username: event.target.value,
                    })
                  }
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor={`${candidate.id}-accessToken`}>
                  {t('import.connect.accessToken')}
                </Label>
                <Input
                  id={`${candidate.id}-accessToken`}
                  type='password'
                  value={candidate.accessToken}
                  onChange={(event) =>
                    updateCandidate(candidate.id, {
                      accessToken: event.target.value,
                    })
                  }
                />
              </div>
              <div className='grid gap-1.5'>
                <Label htmlFor={`${candidate.id}-apiToken`}>
                  {t('import.connect.apiToken')}
                </Label>
                <Input
                  id={`${candidate.id}-apiToken`}
                  value={candidate.apiToken}
                  onChange={(event) =>
                    updateCandidate(candidate.id, {
                      apiToken: event.target.value,
                    })
                  }
                />
              </div>
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

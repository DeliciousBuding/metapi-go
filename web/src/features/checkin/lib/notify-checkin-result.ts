import type { TFunction } from 'i18next'
import type { z } from 'zod'

import { toast } from '@/lib/toast'

import type { triggerCheckinResultSchema } from '../types'

/** One outcome presentation for manual check-in from logs, dialog, or account. */
export function notifyCheckinResult(
  result: z.infer<typeof triggerCheckinResultSchema>,
  t: TFunction
) {
  if (result.status === 'success') {
    toast.success(t('checkin.toast.success'), {
      description: result.reward
        ? t('checkin.toast.successReward', { reward: result.reward })
        : undefined,
    })
  } else if (result.status === 'skipped' || result.skipped) {
    toast.info(t('checkin.toast.skipped'), {
      description: result.message || undefined,
    })
  } else {
    toast.error(t('checkin.toast.failed'), {
      description: result.message || undefined,
    })
  }
}

// metapi-go/features/models/price-compare — provenance grade badge.
// Dual-channel status: icon + text + semantic color, never color-only.

import {
  Activity,
  CheckCircle2,
  Settings,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

import { priceGradeValues, type PriceGrade } from '../types'

const gradeIcon: Record<PriceGrade, LucideIcon> = {
  billing_details: CheckCircle2,
  observed: Activity,
  configured: Settings,
  fallback: TriangleAlert,
}

const gradeVariant: Record<
  PriceGrade,
  'default' | 'secondary' | 'warning' | 'outline'
> = {
  billing_details: 'default',
  observed: 'secondary',
  configured: 'warning',
  fallback: 'outline',
}

function isPriceGrade(value: string): value is PriceGrade {
  return (priceGradeValues as readonly string[]).includes(value)
}

export function PriceGradeBadge({ grade }: { grade: string }) {
  const { t } = useTranslation()
  const normalized: PriceGrade = isPriceGrade(grade) ? grade : 'fallback'
  const Icon = gradeIcon[normalized]
  return (
    <Badge variant={gradeVariant[normalized]}>
      <Icon aria-hidden='true' />
      {t(`priceCompare.grade.${normalized}`)}
    </Badge>
  )
}

import { X } from 'lucide-react'
import { useMemo, useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formLabelIdFor } from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { normalizeModelRules } from './key-form-shared'

type ModelPolicyEditorProps = {
  value: string[]
  onChange: (rules: string[]) => void
  candidateModels?: string[]
  routeGrantCount?: number
} & Omit<ComponentProps<'div'>, 'onChange'>

export function ModelPolicyEditor({
  value,
  onChange,
  candidateModels = [],
  routeGrantCount = 0,
  ...props
}: ModelPolicyEditorProps) {
  const { t } = useTranslation()
  // Composite control: the field is a group (summary + rules + add-rule input +
  // suggestions). The FormControl-injected id / aria-* belong on the group root,
  // named via aria-labelledby -> the FormLabel id; the inner add-rule <Input> is
  // a sub-control and gets its OWN aria-label, never the field-level id (#1300).
  const labelId = props.id ? formLabelIdFor(props.id) : undefined
  const [pendingRule, setPendingRule] = useState('')
  const rules = normalizeModelRules(value)
  const normalizedPendingRule = pendingRule.trim()
  const suggestions = useMemo(() => {
    if (!normalizedPendingRule) return []
    const query = normalizedPendingRule.toLowerCase()
    return candidateModels
      .filter(
        (model) => model.toLowerCase().includes(query) && !rules.includes(model)
      )
      .slice(0, 8)
  }, [candidateModels, normalizedPendingRule, rules])

  function addRule(rawRule: string) {
    const rule = rawRule.trim()
    if (!rule) return
    onChange(normalizeModelRules([...rules, rule]))
    setPendingRule('')
  }

  function removeRule(rule: string) {
    onChange(rules.filter((candidate) => candidate !== rule))
  }

  let summary = t('settings.downstream.keys.models.summary.rules', {
    count: rules.length,
    defaultValue: '{{count}} rules',
  })
  let summaryVariant: 'success' | 'warning' | 'outline' = 'outline'
  if (rules.includes('*')) {
    summary = t('settings.downstream.keys.models.summary.all', {
      defaultValue: 'All models',
    })
    summaryVariant = 'success'
  } else if (rules.length === 0 && routeGrantCount > 0) {
    summary = t('settings.downstream.keys.models.summary.routes', {
      count: routeGrantCount,
      defaultValue: '{{count}} route grants',
    })
  } else if (rules.length === 0) {
    summary = t('settings.downstream.keys.models.summary.none', {
      defaultValue: 'No models authorized',
    })
    summaryVariant = 'warning'
  }

  return (
    <div
      className='space-y-2'
      role='group'
      aria-labelledby={labelId}
      {...props}
    >
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant={summaryVariant} data-testid='model-policy-form-summary'>
          {summary}
        </Badge>
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={() => onChange(['*'])}
        >
          {t('settings.downstream.keys.models.allowAll', {
            defaultValue: 'Allow all',
          })}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={() => onChange([])}
        >
          {t('settings.downstream.keys.models.denyAll', {
            defaultValue: 'Deny all',
          })}
        </Button>
      </div>

      {rules.length > 0 ? (
        <div className='flex flex-wrap gap-1' data-testid='model-policy-rules'>
          {rules.map((rule) => (
            <Badge key={rule} variant='secondary' className='font-mono'>
              {rule}
              <button
                type='button'
                className='focus-visible:ring-focus-ring rounded-full outline-none focus-visible:ring-2'
                aria-label={t(
                  'settings.downstream.keys.models.removeRuleAria',
                  { rule, defaultValue: 'Remove model rule {{rule}}' }
                )}
                onClick={() => removeRule(rule)}
              >
                <X aria-hidden='true' />
              </button>
            </Badge>
          ))}
        </div>
      ) : null}

      <div className='flex gap-2'>
        <Input
          aria-label={t('settings.downstream.keys.models.inputAria', {
            defaultValue: 'Model rule',
          })}
          value={pendingRule}
          onChange={(event) => setPendingRule(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              if (
                event.nativeEvent.isComposing ||
                event.nativeEvent.keyCode === 229
              )
                return
              addRule(pendingRule)
            }
          }}
          placeholder={t('settings.downstream.keys.models.inputPlaceholder', {
            defaultValue: 'gpt-5.5, gpt-*, or re:^claude-',
          })}
          className='font-mono'
        />
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={!normalizedPendingRule}
          onClick={() => addRule(pendingRule)}
        >
          {t('settings.common.add')}
        </Button>
      </div>

      {suggestions.length > 0 ? (
        <div
          className='flex flex-wrap gap-1'
          role='listbox'
          aria-label={t('settings.downstream.keys.models.suggestions', {
            defaultValue: 'Matching models',
          })}
        >
          {suggestions.map((model) => (
            <Button
              key={model}
              type='button'
              role='option'
              aria-selected='false'
              variant='ghost'
              size='xs'
              className='font-mono'
              onClick={() => addRule(model)}
            >
              + <ModelPill model={model} variant='inline' />
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

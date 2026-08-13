// metapi-go/features/import — five-step wizard stepper.
//
// Each step carries one of three visual states: completed (check), current
// (highlighted, aria-current="step"), and upcoming (dimmed). Color is never the
// only signal: completed steps also show a check icon and the current step gets
// a text label, so the states remain distinguishable without color.

import { Check as CheckIcon } from 'lucide-react'

import { cn } from '@/lib/utils'

export type ImportStep = {
  id: string
  label: string
}

type StepperProps = {
  steps: ImportStep[]
  currentIndex: number
}

function stepState(index: number, currentIndex: number): 'current' | 'completed' | 'upcoming' {
  if (index === currentIndex) return 'current'
  if (index < currentIndex) return 'completed'
  return 'upcoming'
}

export function ImportStepper({ steps, currentIndex }: StepperProps) {
  return (
    <ol
      aria-label='Import progress'
      className='flex items-center gap-1.5'
    >
      {steps.map((step, index) => {
        const state = stepState(index, currentIndex)
        const isCompleted = state === 'completed'
        const isCurrent = state === 'current'

        return (
          <li
            key={step.id}
            className='flex min-w-0 flex-1 items-center gap-1.5'
          >
            <div
              aria-current={isCurrent ? 'step' : undefined}
              data-state={state}
              className={cn(
                'flex h-7 min-w-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium',
                isCurrent &&
                  'border-primary bg-primary text-primary-foreground',
                isCompleted &&
                  'border-primary/30 bg-primary/10 text-primary',
                !isCurrent && !isCompleted &&
                  'border-border text-muted-foreground'
              )}
            >
              {isCompleted ? (
                <CheckIcon className='size-3.5' aria-hidden='true' />
              ) : (
                index + 1
              )}
            </div>
            <span
              className={cn(
                'truncate text-xs',
                isCurrent ? 'font-medium text-foreground' : 'text-muted-foreground'
              )}
            >
              {step.label}
            </span>
            {index < steps.length - 1 && (
              <div
                aria-hidden='true'
                className={cn(
                  'h-px min-w-2 flex-1',
                  isCompleted ? 'bg-primary/40' : 'bg-border'
                )}
              />
            )}
          </li>
        )
      })}
    </ol>
  )
}

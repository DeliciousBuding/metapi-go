import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { useRouteDecision } from '../api'
import { routeCheckEndpoints } from '../lib/route-check-endpoints'
import type { RouteDecision, RouteSummaryRow } from '../types'
import { RouteDecisionResult } from './route-decision-result'

const endpoints = routeCheckEndpoints

type CheckInput = { model: string; endpoint: string }

export function RouteDecisionPanel(props: {
  route: RouteSummaryRow
  snapshot: RouteDecision | null
}) {
  const { t } = useTranslation()
  const [submitted, setSubmitted] = useState<CheckInput | null>(null)
  const query = useRouteDecision(submitted)
  const schema = z.object({
    model: z
      .string()
      .trim()
      .min(1, t('tokenRoutes.preview.modelRequired'))
      .refine(
        (model) => !/[*?]/.test(model),
        t('tokenRoutes.preview.concreteModel')
      ),
    endpoint: z.string().min(1),
  })
  const initialModel =
    props.route.displayName ||
    (/[*?^$|()[\]{}]/.test(props.route.modelPattern)
      ? ''
      : props.route.modelPattern)
  const form = useForm<CheckInput>({
    resolver: zodResolver(schema),
    defaultValues: { model: initialModel, endpoint: endpoints[0][0] },
  })
  const check = (input: CheckInput) => {
    if (
      input.model === submitted?.model &&
      input.endpoint === submitted.endpoint
    ) {
      void query.refetch()
    } else {
      setSubmitted(input)
    }
  }
  return (
    <div className='min-w-0 space-y-5'>
      <section
        className='space-y-3'
        aria-label={t('tokenRoutes.preview.title')}
      >
        <div>
          <h3 className='text-sm font-medium'>
            {t('tokenRoutes.preview.title')}
          </h3>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('tokenRoutes.preview.boundary')}
          </p>
        </div>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(check)} className='space-y-3'>
            <FormField
              control={form.control}
              name='model'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.preview.model')}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      placeholder={t('tokenRoutes.preview.modelPlaceholder')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='endpoint'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.preview.endpoint')}</FormLabel>
                  <Select
                    items={endpoints.map(([value, label]) => ({
                      value,
                      label,
                    }))}
                    value={field.value}
                    onValueChange={(value) => field.onChange(value)}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {endpoints.map(([path, label]) => (
                        <SelectItem key={path} value={path}>
                          {label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            <Button type='submit' variant='outline' disabled={query.isFetching}>
              {query.isFetching
                ? t('tokenRoutes.preview.checking')
                : t('tokenRoutes.preview.check')}
            </Button>
          </form>
        </Form>
        {query.isError && (
          <p role='alert' className='text-destructive text-xs'>
            {t('tokenRoutes.preview.failed')}
          </p>
        )}
        {query.data && !query.isError && submitted && (
          <div
            aria-live='polite'
            aria-busy={query.isFetching}
            className='space-y-2'
          >
            <p className='text-muted-foreground text-xs break-words'>
              {t('tokenRoutes.preview.checkedInput', submitted)}
            </p>
            {query.data.routeId != null &&
              query.data.routeId !== props.route.id && (
                <p className='text-xs'>
                  {t('tokenRoutes.preview.otherRoute', {
                    id: query.data.routeId,
                  })}
                </p>
              )}
            <RouteDecisionResult decision={query.data} />
          </div>
        )}
      </section>
      <section
        className='space-y-2 border-t pt-4'
        aria-label={t('tokenRoutes.detail.decisionSnapshot')}
      >
        <h3 className='text-sm font-medium'>
          {t('tokenRoutes.detail.decisionSnapshot')}
        </h3>
        <p className='text-muted-foreground text-xs'>
          {t('tokenRoutes.preview.snapshotBoundary')}
        </p>
        {props.snapshot ? (
          <RouteDecisionResult
            decision={props.snapshot}
            snapshotAt={props.route.decisionRefreshedAt}
          />
        ) : (
          <p className='text-muted-foreground text-xs'>
            {t('tokenRoutes.detail.decisionEmpty')}
          </p>
        )}
      </section>
    </div>
  )
}

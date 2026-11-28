import { ChevronDown } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Form,
  FormDescription,
  FormItem,
  FormLabel,
} from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { RouteMode, RouteSummaryRow } from '../types'
import { RouteAdvancedFields } from './route-advanced-fields'
import {
  RouteChannelDraftField,
  type RouteAccountOption,
} from './route-channel-draft-field'
import { RouteChannelEditor } from './route-channel-editor'
import { RoutePatternField, GroupModeFields } from './route-match-fields'
import { useRouteForm, type RouteFormOptions } from './use-route-form'

export type { RouteAccountOption } from './route-channel-draft-field'

interface RouteFormDialogProps extends RouteFormOptions {
  availableRoutes: RouteSummaryRow[]
  accountOptions: RouteAccountOption[]
}

export function RouteFormDialog({
  open,
  onOpenChange,
  mode,
  route,
  availableRoutes,
  accountOptions,
  chainContext,
}: RouteFormDialogProps) {
  const { t } = useTranslation()
  const {
    isEdit,
    form,
    handleOpenChange,
    guard,
    routeMode,
    advancedOpen,
    setAdvancedOpen,
    isInitialized,
    onSubmit,
    onInvalid,
    isSubmitting,
  } = useRouteForm({ open, onOpenChange, mode, route, chainContext })
  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-xl'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b px-5 py-5 sm:px-6'>
          <SheetTitle>
            {isEdit
              ? t('tokenRoutes.form.editTitle')
              : t('tokenRoutes.form.addTitle')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('tokenRoutes.form.editDescription')
              : t('tokenRoutes.form.addDescription')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='route-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            inert={!isInitialized ? true : undefined}
            aria-busy={!isInitialized}
            className='min-h-0 flex-1 space-y-6 overflow-y-auto px-5 py-5 sm:px-6'
          >
            <FormItem>
              <FormLabel>{t('tokenRoutes.form.routeType')}</FormLabel>
              <Tabs
                value={routeMode}
                onValueChange={(value) =>
                  form.setValue('routeMode', value as RouteMode, {
                    shouldDirty: true,
                  })
                }
              >
                <TabsList>
                  <TabsTrigger value='explicit_group'>
                    {t('tokenRoutes.form.modeGroup')}
                  </TabsTrigger>
                  <TabsTrigger value='pattern'>
                    {t('tokenRoutes.form.modePattern')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
              <FormDescription>
                {routeMode === 'explicit_group'
                  ? t('tokenRoutes.form.modeGroupHint')
                  : t('tokenRoutes.form.modePatternHint')}
              </FormDescription>
            </FormItem>
            {isEdit && route && route.id > 0 && (
              <RouteChannelEditor routeId={route.id} />
            )}
            {routeMode === 'pattern' ? (
              <>
                <RoutePatternField form={form} />
                <RouteChannelDraftField
                  form={form}
                  accountOptions={accountOptions}
                  isEdit={isEdit}
                  chainContext={isEdit ? undefined : chainContext}
                />
              </>
            ) : (
              <GroupModeFields form={form} availableRoutes={availableRoutes} />
            )}
            <Collapsible
              open={advancedOpen}
              onOpenChange={setAdvancedOpen}
              className='border-t pt-4'
            >
              <CollapsibleTrigger className='group focus-visible:ring-ring flex w-full items-center justify-between gap-3 rounded-md py-1 text-left outline-none focus-visible:ring-2'>
                <span className='min-w-0 space-y-1'>
                  <span className='block text-sm font-semibold'>
                    {t('tokenRoutes.form.advancedTitle')}
                  </span>
                  <span className='text-muted-foreground block text-xs leading-5'>
                    {t('tokenRoutes.form.advancedHint')}
                  </span>
                </span>
                <ChevronDown
                  className='text-muted-foreground size-4 shrink-0 transition-transform group-aria-expanded:rotate-180 motion-reduce:transition-none'
                  aria-hidden='true'
                />
              </CollapsibleTrigger>
              <CollapsibleContent keepMounted>
                <RouteAdvancedFields form={form} />
              </CollapsibleContent>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className='bg-background shrink-0 flex-row justify-end border-t px-5 py-4 sm:px-6'>
          <Button
            variant='outline'
            className='flex-1 sm:flex-none'
            onClick={() => handleOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='route-form'
            className='flex-1 sm:flex-none'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Spinner />}
            {isEdit ? t('tokenRoutes.form.save') : t('tokenRoutes.form.create')}
          </Button>
        </SheetFooter>
      </SheetContent>
      {guard}
    </Sheet>
  )
}

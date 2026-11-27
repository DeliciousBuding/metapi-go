import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from '@/components/ui/form'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import {
  gatewayUsersApi,
  type GatewayUser,
  type GatewayUserGroup,
} from '@/lib/api/gateway-users'

import { useAdminWrite } from './api'
import { BooleanField, TextField } from './editor-fields'
import { WriteError } from './feedback'

export { ActivationDialog } from './activation-dialog'
export type { EditorState } from './editor-fields'
export { PricingEditor } from './group-pricing-editor'
export { BudgetDialog, BudgetEditor } from './user-budget-editor'

export function IdentityEditor({
  kind,
  user,
  group,
  groups,
  close,
}: {
  kind: 'user' | 'group'
  user?: GatewayUser
  group?: GatewayUserGroup
  groups: GatewayUserGroup[]
  close: () => void
}) {
  const { t } = useTranslation()
  const editing = Boolean(user || group)
  const titleKeys = {
    user: { create: 'userAdmin.createUser', edit: 'userAdmin.editUser' },
    group: { create: 'userAdmin.createGroup', edit: 'userAdmin.editGroup' },
  }
  const schema = z.object({
    identifier: editing
      ? z.string()
      : z
          .string()
          .trim()
          .regex(/^[a-z0-9_-]{1,64}$/, t('userAdmin.identifierHint')),
    name: z
      .string()
      .trim()
      .max(100, t('userAdmin.nameHint'))
      .refine(
        (value) => kind === 'user' || value.length > 0,
        t('userAdmin.nameHint')
      ),
    groupId: z.number().int().nonnegative(),
    enabled: z.boolean(),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      identifier: user?.username ?? group?.slug ?? '',
      name: user?.displayName ?? group?.name ?? '',
      groupId: user?.groupId ?? 0,
      enabled: user?.enabled ?? group?.enabled ?? true,
    },
  })
  const mutation = useAdminWrite(
    async (value: z.infer<typeof schema>, token) => {
      if (kind === 'user') {
        const body = {
          displayName: value.name,
          groupId: value.groupId,
          enabled: value.enabled,
        }
        return user
          ? gatewayUsersApi.updateUser(user.id, body, token)
          : gatewayUsersApi.createUser(
              { ...body, username: value.identifier },
              token
            )
      }
      const body = { name: value.name, enabled: value.enabled }
      return group
        ? gatewayUsersApi.updateGroup(group.id, body, token)
        : gatewayUsersApi.createGroup(
            { ...body, slug: value.identifier },
            token
          )
    },
    () => {
      form.reset()
      close()
    }
  )
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onOpenChange: (open) => {
      if (!open) close()
    },
  })
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!mutation.isPending) handleOpenChange(open)
        }}
      >
        <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {t(titleKeys[kind][editing ? 'edit' : 'create'])}
            </DialogTitle>
            <DialogDescription>
              {t(
                kind === 'user'
                  ? 'userAdmin.userBoundary'
                  : 'userAdmin.groupBoundary'
              )}
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit((value) => mutation.submit(value))}
              className='space-y-4'
            >
              <TextField
                control={form.control}
                name='identifier'
                label={t(
                  kind === 'user' ? 'userAdmin.username' : 'userAdmin.slug'
                )}
                disabled={editing}
              />
              {!editing && (
                <p className='text-muted-foreground text-xs'>
                  {t('userAdmin.identifierHint')}
                </p>
              )}
              <TextField
                control={form.control}
                name='name'
                label={t(
                  kind === 'user'
                    ? 'userAdmin.displayName'
                    : 'userAdmin.groupName'
                )}
              />
              {kind === 'user' && (
                <FormField
                  control={form.control}
                  name='groupId'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('userAdmin.group')}</FormLabel>
                      <Select
                        value={String(field.value)}
                        onValueChange={(value) => field.onChange(Number(value))}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue>
                              {field.value === 0
                                ? t('userAdmin.defaultGroup')
                                : (groups.find(
                                    (item) => item.id === field.value
                                  )?.name ?? `#${field.value}`)}
                            </SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {!editing && (
                            <SelectItem value='0'>
                              {t('userAdmin.defaultGroup')}
                            </SelectItem>
                          )}
                          {groups.map((item) => (
                            <SelectItem key={item.id} value={String(item.id)}>
                              {item.name}
                              {!item.enabled
                                ? ` · ${t('userAdmin.disabled')}`
                                : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              {kind === 'user' && !editing && (
                <Notice tone='warning'>{t('userAdmin.defaultWarning')}</Notice>
              )}
              <BooleanField
                control={form.control}
                name='enabled'
                label={t('userAdmin.enabled')}
              />
              <WriteError error={mutation.error} />
              <div className='flex justify-end gap-2'>
                <Button
                  variant='outline'
                  type='button'
                  onClick={() => handleOpenChange(false)}
                  disabled={mutation.isPending}
                >
                  {t('userAdmin.cancel')}
                </Button>
                <Button type='submit' disabled={mutation.isPending}>
                  {t(
                    mutation.isPending ? 'userAdmin.saving' : 'userAdmin.save'
                  )}
                </Button>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
      {guard}
    </>
  )
}

import { createFileRoute } from '@tanstack/react-router'

import { userAdminSearchSchema } from '@/features/user-administration/search'
import { UserAdministrationPage } from '@/features/user-administration/user-administration-page'

export const Route = createFileRoute('/_authenticated/users')({
  validateSearch: userAdminSearchSchema,
  staticData: { title: 'userAdmin.title' },
  component: UserAdministrationPage,
})

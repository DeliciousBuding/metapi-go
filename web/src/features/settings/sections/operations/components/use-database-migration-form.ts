import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'

import { api } from '@/lib/api'

import {
  runtimeDatabaseQueryKeys,
  type RuntimeDatabaseConfig,
} from './database-shared'

const migrationSchema = z.object({
  dialect: z.enum(['sqlite', 'postgres']),
  connectionString: z.string(),
  overwrite: z.boolean(),
  ssl: z.boolean(),
})

export type MigrationFormValues = z.infer<typeof migrationSchema>

const DEFAULT_VALUES: MigrationFormValues = {
  dialect: 'postgres',
  connectionString: '',
  overwrite: true,
  ssl: false,
}

export function useDatabaseMigrationForm() {
  const configQuery = useQuery<RuntimeDatabaseConfig>({
    queryKey: runtimeDatabaseQueryKeys.all,
    queryFn: async () =>
      (await api.getRuntimeDatabaseConfig()) as RuntimeDatabaseConfig,
    staleTime: 30 * 1000,
  })

  const form = useForm<MigrationFormValues>({
    resolver: zodResolver(migrationSchema) as never,
    defaultValues: DEFAULT_VALUES,
  })

  // Sensible default target: migrate away from the live dialect. Skipped once
  // the user has touched the form so their edits are never clobbered.
  useEffect(() => {
    if (!configQuery.data || form.formState.isDirty) return
    const liveDialect = configQuery.data.active?.dialect
    const targetDialect = liveDialect === 'postgres' ? 'sqlite' : 'postgres'
    form.setValue('dialect', targetDialect, { shouldDirty: false })
    if (targetDialect === 'sqlite') {
      form.setValue('ssl', false, { shouldDirty: false })
    }
  }, [configQuery.data, form])
  return { configQuery, form }
}

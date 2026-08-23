// metapi-go/features/settings/sections/operations/components — shared
// runtime-database types + query key used by both the runtime-config section
// and the migration section. Living in its own module avoids a component
// import cycle (database-section ↔ database-migration-section).

export type RuntimeDatabaseConfig = {
  active?: { dialect: string; connection: string; ssl?: boolean }
  saved?: {
    dialect: 'sqlite' | 'postgres'
    connectionStringMasked?: string
    hasConnectionString?: boolean
    ssl?: boolean
  }
  restartRequired?: boolean
}

export const runtimeDatabaseQueryKeys = {
  all: ['runtime-database-config'] as const,
}

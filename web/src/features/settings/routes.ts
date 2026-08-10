// metapi-go/features/settings — route registration.
//
// metapi-go uses TanStack Router FILE-BASED routing (the `routes/` tree),
// auto-generated into `routeTree.gen.ts` by the `@tanstack/router-plugin`
// configured in rsbuild.config.ts. The actual route files live under
//   routes/_authenticated/settings/<subarea>/{$section,index}.tsx
// and are thin (~10 lines each): they read the `$section` param, validate it
// against the subarea's `sectionIds`, and render
//   <SettingsPage subarea={...} activeSection={section} />.
//
// This module centralizes the subarea→registry config + validation helpers
// the route files import. It does NOT register routes itself (file-based
// routing does that); it is the single source of truth for which subareas +
// sections exist, so route files stay declarative.
//
// --- Route-file template (per subarea) -------------------------------------
//
//   // routes/_authenticated/settings/general/$section.tsx
//   import { createFileRoute, redirect } from '@tanstack/react-router'
//   import {
//     SettingsPage,
//     getSettingsSubarea,
//     isValidSection,
//   } from '@/features/settings'
//   import { GENERAL_DEFAULT_SECTION } from '@/features/settings/sections/general'
//
//   function GeneralSettingsRoute() {
//     const { section } = Route.useParams()
//     return (
//       <SettingsPage
//         subarea={getSettingsSubarea('general')!}
//         activeSection={section}
//       />
//     )
//   }
//
//   export const Route = createFileRoute(
//     '/_authenticated/settings/general/$section',
//   )({
//     beforeLoad: ({ params }) => {
//       if (!isValidSection('general', params.section)) {
//         throw redirect({
//           to: '/settings/general/$section',
//           params: { section: GENERAL_DEFAULT_SECTION },
//         })
//       }
//     },
//     component: GeneralSettingsRoute,
//   })
//
//   // routes/_authenticated/settings/general/index.tsx
//   // (no-section redirect → default section)
//   import { createFileRoute, redirect } from '@tanstack/react-router'
//   import { GENERAL_DEFAULT_SECTION } from '@/features/settings/sections/general'
//   export const Route = createFileRoute('/_authenticated/settings/general/')({
//     beforeLoad: () => {
//       throw redirect({
//         to: '/settings/general/$section',
//         params: { section: GENERAL_DEFAULT_SECTION },
//       })
//     },
//   })
//
//   // routes/_authenticated/settings/index.tsx
//   // (/settings → first subarea)
//   import { createFileRoute, redirect } from '@tanstack/react-router'
//   export const Route = createFileRoute('/_authenticated/settings/')({
//     beforeLoad: () => {
//       throw redirect({ to: '/settings/general' })
//     },
//   })
//
// TODO(phase 2 wiring): add the ~11 route files above so /settings/* is
// navigable. The main sidebar (system-settings.config.ts) already links to
// /settings/<subarea>; the routes close the loop.

export {
  SETTINGS_SUBAREAS,
  SETTINGS_SUBAREA_IDS,
  getSettingsSubarea,
  resolveDefaultSection,
  isValidSection,
} from './config/settings-config'

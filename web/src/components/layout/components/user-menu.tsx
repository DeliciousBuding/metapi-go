// metapi-go/layout — header user menu.
//
// Right-most header control: a ghost icon button (matching the header's
// language/theme controls) opening a dropdown with the product version,
// About + documentation links, and sign-out. Sign-out clears the persisted
// session (localStorage token + monitor cookie) and the in-memory auth
// store, then lands on the sign-in page. Kept as an independent component
// so the header chrome and the auth flow never import each other.

import { Link, useNavigate } from '@tanstack/react-router'
import { BookOpen, CircleUserRound, Info, LogOut } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ABOUT_INFO } from '@/features/about/api'
import { api } from '@/lib/api'
import { clearAuthSession } from '@/lib/auth-session'
import { useAuthStore } from '@/stores/auth-store'

export function UserMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()

  const handleSignOut = () => {
    // Clear the HttpOnly `meta_monitor_auth` cookie server-side while the
    // Bearer token is still valid — the JS cookie clear inside
    // clearAuthSession cannot touch HttpOnly cookies. Fire-and-forget: a
    // failed monitor cleanup must not block sign-out.
    void api.clearMonitorSession().catch(() => {})
    clearAuthSession()
    useAuthStore.getState().auth.reset()
    navigate({ to: '/sign-in' })
  }

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('userMenu.trigger')}
          />
        }
      >
        <CircleUserRound className='size-4' />
        <span className='sr-only'>{t('userMenu.trigger')}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-56'>
        <DropdownMenuGroup>
          <DropdownMenuLabel>
            {ABOUT_INFO.projectName} v{ABOUT_INFO.version}
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem render={(props) => <Link to='/about' {...props} />}>
            <Info />
            {t('userMenu.about')}
          </DropdownMenuItem>
          <DropdownMenuItem
            render={(props) => (
              <a
                href={ABOUT_INFO.homepage}
                target='_blank'
                rel='noopener noreferrer'
                {...props}
              />
            )}
          >
            <BookOpen />
            {t('userMenu.documentation')}
          </DropdownMenuItem>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant='destructive' onSelect={handleSignOut}>
            <LogOut />
            {t('userMenu.signOut')}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

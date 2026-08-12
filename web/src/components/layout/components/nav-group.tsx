// metapi-go/layout — nav-group ported from newapi. AGPL header stripped.
// Dropped ChatPresetsItem handling (metapi has no chat). Renders NavLink and
// NavCollapsible items, with a collapsed-state dropdown for desktop icon mode.

import { Link, useLocation, type LinkProps } from '@tanstack/react-router'
import { ChevronRight } from 'lucide-react'
import { type ReactNode, useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from '@/components/ui/sidebar'
import { cn } from '@/lib/utils'

import { checkIsActive } from '../lib/url-utils'
import type {
  NavCollapsible,
  NavLink,
  NavGroup as NavGroupProps,
} from '../types'

/**
 * Sidebar navigation group component
 * Renders a group of navigation items, supporting regular links and collapsible submenus
 */
export function NavGroup({ title, items }: NavGroupProps) {
  const { state, isMobile } = useSidebar()
  const href = useLocation({ select: (location) => location.href })
  const { t } = useTranslation()

  return (
    <SidebarGroup className='px-2 py-1'>
      <SidebarGroupLabel className='text-muted-foreground/70 px-2 text-[11px] font-medium tracking-wider uppercase'>
        {t(title)}
      </SidebarGroupLabel>
      <SidebarMenu>
        {items.map((item) => {
          const key = `${item.title}-${item.url}`

          // If no sub-items, render regular link
          if (!item.items) {
            return (
              <SidebarMenuLink key={key} item={item as NavLink} href={href} />
            )
          }

          // In collapsed state on non-mobile, render dropdown menu
          if (state === 'collapsed' && !isMobile) {
            return (
              <SidebarMenuCollapsedDropdown
                key={key}
                item={item as NavCollapsible}
                href={href}
              />
            )
          }

          // Render collapsible menu
          return (
            <SidebarMenuCollapsible
              key={key}
              item={item as NavCollapsible}
              href={href}
            />
          )
        })}
      </SidebarMenu>
    </SidebarGroup>
  )
}

/**
 * Navigation badge component
 */
function NavBadge({ children }: { children: ReactNode }) {
  return <Badge className='shrink-0 px-1 py-0 text-xs'>{children}</Badge>
}

/**
 * TanStack Router Link rendered through Base UI's `render` prop.
 *
 * Base UI's useRender clones the render element with its own props merged in
 * (including `children` — a React element). TanStack Link then treats the
 * whole props object as navigate options, so the Fiber-backed children leak
 * into `router.navigate` and blow up when the router serializes options
 * ("Converting circular structure to JSON"). This component picks out only
 * the DOM-safe props Base UI injects (className / data-* / pointer events /
 * ref) and keeps React elements (children) out of the prop bag entirely.
 */
const BASE_UI_SAFE_PROPS = [
  'className',
  'id',
  'style',
  'ref',
  'data-slot',
  'data-sidebar',
  'data-size',
  'data-trigger-disabled',
  'data-base-ui-tooltip-trigger',
  'onPointerDown',
  'onPointerEnter',
  'onPointerMove',
  'onPointerUp',
  'onMouseDown',
  'onMouseMove',
  'onMouseLeave',
  'onFocus',
  'onBlur',
  'onMouseOver',
  'onClick',
] as const

function SidebarNavLink({
  to,
  renderProps,
  activeOptions,
}: {
  to: LinkProps['to'] | (string & {})
  renderProps: React.HTMLAttributes<HTMLAnchorElement> & {
    ref?: React.Ref<HTMLAnchorElement>
  }
  activeOptions?: LinkProps['activeOptions']
}) {
  const { setOpenMobile } = useSidebar()
  const propsBag = renderProps as unknown as Record<string, unknown>
  const safeProps: Record<string, unknown> = {}
  for (const key of BASE_UI_SAFE_PROPS) {
    if (key in propsBag) safeProps[key] = propsBag[key]
  }
  const baseOnClick = propsBag.onClick as
    | ((event: React.MouseEvent<HTMLAnchorElement>) => void)
    | undefined
  const children = propsBag.children as ReactNode
  return (
    <Link
      to={to}
      activeOptions={activeOptions}
      {...safeProps}
      onClick={(event) => {
        setOpenMobile(false)
        baseOnClick?.(event)
      }}
    >
      {children}
    </Link>
  )
}

/**
 * Sidebar menu link item
 */
function SidebarMenuLink({ item, href }: { item: NavLink; href: string }) {
  const { t } = useTranslation()
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        isActive={checkIsActive(href, item)}
        tooltip={t(item.title)}
        render={(props) => (
          <SidebarNavLink
            to={item.url}
            renderProps={props}
            activeOptions={item.activeOptions}
          />
        )}
      >
        {item.icon && <item.icon className='shrink-0' />}
        <span className='min-w-0 flex-1 truncate'>{t(item.title)}</span>
        {item.badge && <NavBadge>{item.badge}</NavBadge>}
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

/**
 * Sidebar collapsible menu item
 */
function SidebarMenuCollapsible({
  item,
  href,
}: {
  item: NavCollapsible
  href: string
}) {
  const { t } = useTranslation()
  const isSubItemActive = checkIsActive(href, item)
  const [isOpen, setIsOpen] = useState(() => isSubItemActive)

  useEffect(() => {
    if (isSubItemActive) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setIsOpen(true)
    }
  }, [isSubItemActive])

  return (
    <Collapsible
      open={isOpen}
      onOpenChange={setIsOpen}
      className='group/collapsible'
      render={<SidebarMenuItem />}
    >
      <CollapsibleTrigger
        className='group/collapsible-trigger'
        render={<SidebarMenuButton tooltip={t(item.title)} />}
      >
        {item.icon && <item.icon className='shrink-0' />}
        <span className='min-w-0 flex-1 truncate'>{t(item.title)}</span>
        {item.badge && <NavBadge>{item.badge}</NavBadge>}
        <ChevronRight className='ms-auto size-4 shrink-0 transition-transform duration-200 group-data-[panel-open]/collapsible-trigger:rotate-90' />
      </CollapsibleTrigger>
      <CollapsibleContent className='CollapsibleContent'>
        <SidebarMenuSub>
          {item.items.map((subItem) => (
            <SidebarMenuSubItem key={subItem.title}>
              <SidebarMenuSubButton
                isActive={checkIsActive(href, subItem)}
                render={(props) => (
                  <SidebarNavLink
                    to={subItem.url}
                    renderProps={props}
                    activeOptions={subItem.activeOptions}
                  />
                )}
              >
                {subItem.icon && <subItem.icon className='shrink-0' />}
                <span className='min-w-0 flex-1 truncate'>
                  {t(subItem.title)}
                </span>
                {subItem.badge && <NavBadge>{subItem.badge}</NavBadge>}
              </SidebarMenuSubButton>
            </SidebarMenuSubItem>
          ))}
        </SidebarMenuSub>
      </CollapsibleContent>
    </Collapsible>
  )
}

/**
 * Sidebar dropdown menu item when collapsed
 */
function SidebarMenuCollapsedDropdown({
  item,
  href,
}: {
  item: NavCollapsible
  href: string
}) {
  const { t } = useTranslation()
  return (
    <SidebarMenuItem>
      <DropdownMenu>
        <DropdownMenuTrigger
          className='group/dropdown-trigger'
          render={
            <SidebarMenuButton
              tooltip={t(item.title)}
              isActive={checkIsActive(href, item)}
            />
          }
        >
          {item.icon && <item.icon className='shrink-0' />}
          <span className='min-w-0 flex-1 truncate'>{t(item.title)}</span>
          {item.badge && <NavBadge>{item.badge}</NavBadge>}
          <ChevronRight className='ms-auto size-4 shrink-0 transition-transform duration-200 group-data-[popup-open]/dropdown-trigger:rotate-90' />
        </DropdownMenuTrigger>
        <DropdownMenuContent side='right' align='start' sideOffset={4}>
          <DropdownMenuGroup>
            <DropdownMenuLabel>
              {t(item.title)} {item.badge ? `(${item.badge})` : ''}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            {item.items.map((sub) => (
              <DropdownMenuItem
                key={`${sub.title}-${sub.url}`}
                render={(props) => (
                  <SidebarNavLink
                    to={sub.url}
                    activeOptions={sub.activeOptions}
                    renderProps={{
                      ...props,
                      className: cn(
                        props.className as string | undefined,
                        checkIsActive(href, sub) ? 'bg-secondary' : undefined
                      ),
                    }}
                  />
                )}
              >
                {sub.icon && <sub.icon />}
                <span className='max-w-52 text-wrap'>{t(sub.title)}</span>
                {sub.badge && (
                  <span className='ms-auto text-xs'>{sub.badge}</span>
                )}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
    </SidebarMenuItem>
  )
}

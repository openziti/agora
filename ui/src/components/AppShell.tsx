import type { ReactNode } from 'react';
import { useEffect, useState } from 'react';
import {
  ArrowLeftRight,
  BookOpen,
  ChevronsLeft,
  ChevronsRight,
  FileCheck2,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  ScrollText,
  ShieldCheck,
  Sun,
  X,
} from 'lucide-react';
import { useNavigate } from 'react-router';

import { BrandMark, type Product } from './BrandMark';
import { NavTabs, type NavTab } from './NavTabs';
import { StatusPill, type StatusPillStatus } from './StatusPill';
import { UserBadge } from './UserBadge';
import { logout } from '../lib/api';
import { clearAuthState, useAuthState } from '../lib/auth-state';

export type AppShellProps = {
  product?: Product;
  organizationName: string;
  activeTab: string;
  navItems?: NavTab[];
  status?: StatusPillStatus;
  statusLabel?: string;
  userInitials?: string;
  userLabel?: string;
  onTabChange?: (id: string) => void;
  fullHeight?: boolean;
  children: ReactNode;
};

// Nav items rendered in the mobile hamburger drawer.
const DRAWER_NAV_ITEMS = [
  { id: 'dashboard',  label: 'Dashboard',  Icon: LayoutDashboard },
  { id: 'sessions',   label: 'Sessions',   Icon: ArrowLeftRight  },
  { id: 'workgroups', label: 'Workgroups', Icon: ShieldCheck     },
  { id: 'catalog',    label: 'Catalog',    Icon: BookOpen        },
  { id: 'contracts',  label: 'Contracts',  Icon: FileCheck2      },
  { id: 'audit',      label: 'Audit',      Icon: ScrollText      },
] as const;

export function AppShell({
  product = 'agora',
  organizationName,
  activeTab,
  navItems,
  status = 'success',
  statusLabel = 'Connected',
  userInitials,
  userLabel,
  onTabChange,
  children,
}: AppShellProps) {
  const navigate = useNavigate();
  const { account } = useAuthState();
  const [loggingOut, setLoggingOut] = useState(false);
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem('agora-sidebar-collapsed') === 'true',
  );
  const [isDark, setIsDark] = useState(
    () => document.documentElement.classList.contains('dark'),
  );
  const [drawerOpen, setDrawerOpen] = useState(false);

  // Track viewport size to drive responsive behaviour.
  // mobile  (< 768px / md): sidebar hidden, hamburger drawer shown.
  // tablet  (768–1023px / md–lg): sidebar auto-collapsed to icon-only.
  // desktop (≥ 1024px / lg): user-controlled via toggle.
  const [isMobile, setIsMobile] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(max-width: 767px)').matches,
  );
  const [isTablet, setIsTablet] = useState(
    () =>
      typeof window !== 'undefined' &&
      window.matchMedia('(min-width: 768px) and (max-width: 1023px)').matches,
  );

  useEffect(() => {
    const mqMobile = window.matchMedia('(max-width: 767px)');
    const mqTablet = window.matchMedia('(min-width: 768px) and (max-width: 1023px)');
    const onMobile = (e: MediaQueryListEvent) => setIsMobile(e.matches);
    const onTablet = (e: MediaQueryListEvent) => setIsTablet(e.matches);
    mqMobile.addEventListener('change', onMobile);
    mqTablet.addEventListener('change', onTablet);
    return () => {
      mqMobile.removeEventListener('change', onMobile);
      mqTablet.removeEventListener('change', onTablet);
    };
  }, []);

  // Close drawer on Escape key.
  useEffect(() => {
    if (!drawerOpen) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') setDrawerOpen(false);
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [drawerOpen]);

  // Tablet always shows icon-only; desktop respects the user's stored preference.
  const effectiveCollapsed = isTablet || collapsed;

  const displayUserLabel = account?.email ?? userLabel;
  const displayUserInitials = account ? initialsForEmail(account.email) : userInitials;

  function toggleDark() {
    const next = !isDark;
    document.documentElement.classList.toggle('dark', next);
    localStorage.setItem('agora-theme', next ? 'dark' : 'light');
    setIsDark(next);
  }

  async function handleLogout() {
    if (loggingOut) return;
    setLoggingOut(true);
    try {
      await logout();
    } finally {
      clearAuthState();
      setLoggingOut(false);
      navigate('/login', { replace: true });
    }
  }

  function handleDrawerNav(id: string) {
    setDrawerOpen(false);
    onTabChange?.(id);
  }

  return (
    <div className="bg-page text-text">
      {/* ── Top header ─────────────────────────────────────────────────────── */}
      <header className="fixed left-0 right-0 top-0 z-[100] flex h-[60px] items-center justify-between border-b border-border bg-panel px-4">
        <BrandMark product={product} />

        {isMobile ? (
          /* Mobile: hamburger only */
          <button
            type="button"
            className="inline-flex size-9 items-center justify-center rounded-pill text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            onClick={() => setDrawerOpen(true)}
            aria-label="Open navigation menu"
            aria-expanded={drawerOpen}
          >
            <Menu size={22} aria-hidden="true" />
          </button>
        ) : (
          /* Tablet + desktop: full header controls */
          <div className="flex items-center gap-3">
            <StatusPill status={status} label={statusLabel} className="max-w-60" />
            <UserBadge
              email={account?.email}
              initials={displayUserInitials}
              label={displayUserLabel}
              loggingOut={loggingOut}
              isDark={isDark}
              onLogout={handleLogout}
              onToggleDark={toggleDark}
              onSetupNewOrg={() => { navigate('/setup'); }}
            />
          </div>
        )}
      </header>

      {/* ── Sidebar (tablet + desktop) ─────────────────────────────────────── */}
      {!isMobile && (
        <aside
          className={[
            'fixed left-0 top-[60px] z-20 flex h-[calc(100vh-60px)] flex-col border-r border-border bg-panel',
            'overflow-hidden transition-[width] duration-300 ease-in-out',
            effectiveCollapsed ? 'w-[65px]' : 'w-64',
          ].join(' ')}
        >
          <div className="flex flex-1 flex-col overflow-y-auto px-2 py-3">
            <NavTabs
              items={navItems}
              activeId={activeTab}
              onTabChange={onTabChange}
              collapsed={effectiveCollapsed}
            />
          </div>

          <div className="border-t border-border p-2">
            <button
              type="button"
              className="flex w-full items-center gap-3 rounded-[5px] px-3 py-2 text-[0.875rem] text-text-mute-strong transition-colors hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
              onClick={() =>
                setCollapsed((c) => {
                  localStorage.setItem('agora-sidebar-collapsed', String(!c));
                  return !c;
                })
              }
              title={effectiveCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {effectiveCollapsed ? (
                <ChevronsRight size={20} className="shrink-0" aria-hidden="true" />
              ) : (
                <>
                  <ChevronsLeft size={20} className="shrink-0" aria-hidden="true" />
                  <span>Collapse</span>
                </>
              )}
            </button>
          </div>
        </aside>
      )}

      {/* ── Main content ───────────────────────────────────────────────────── */}
      <main
        className={[
          'min-h-[calc(100vh-60px)] overflow-y-auto p-6',
          'mt-[60px] transition-[margin-left] duration-300 ease-in-out',
          isMobile ? 'ml-0' : effectiveCollapsed ? 'ml-[65px]' : 'ml-64',
        ].join(' ')}
      >
        {children}
      </main>

      {/* ── Mobile hamburger drawer ─────────────────────────────────────────── */}
      {isMobile && (
        <>
          {/* Backdrop */}
          <div
            className={[
              'fixed inset-0 z-[200] bg-text/40 transition-opacity duration-300',
              drawerOpen ? 'opacity-100' : 'pointer-events-none opacity-0',
            ].join(' ')}
            aria-hidden="true"
            onClick={() => setDrawerOpen(false)}
          />

          {/* Drawer panel */}
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Navigation menu"
            className={[
              'fixed left-0 top-0 z-[201] flex h-full w-72 flex-col bg-panel shadow-xl',
              'transition-transform duration-300 ease-in-out',
              drawerOpen ? 'translate-x-0' : '-translate-x-full',
            ].join(' ')}
          >
            {/* Drawer header */}
            <div className="flex h-[60px] shrink-0 items-center justify-between border-b border-border px-4">
              <BrandMark product={product} />
              <button
                type="button"
                className="inline-flex size-9 items-center justify-center rounded-pill text-text-mute hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
                onClick={() => setDrawerOpen(false)}
                aria-label="Close menu"
              >
                <X size={20} aria-hidden="true" />
              </button>
            </div>

            {/* Account section */}
            <div className="shrink-0 border-b border-border px-4 py-4">
              <div className="flex flex-col items-start gap-3">
                <StatusPill status={status} label={statusLabel} />
              </div>
            </div>

            {/* Navigation links */}
            <nav
              className="flex flex-1 flex-col overflow-y-auto px-2 py-3"
              aria-label="Primary navigation"
            >
              {DRAWER_NAV_ITEMS.map(({ id, label, Icon }) => {
                const isActive = id === activeTab;
                return (
                  <button
                    key={id}
                    type="button"
                    className={[
                      'flex w-full items-center gap-3 rounded-[5px] px-3 py-2.5 text-[0.875rem] transition-colors',
                      'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
                      isActive
                        ? 'bg-panel-subtle font-bold text-text'
                        : 'font-normal text-text-mute hover:bg-panel-subtle',
                    ].join(' ')}
                    aria-current={isActive ? 'page' : undefined}
                    onClick={() => handleDrawerNav(id)}
                  >
                    <Icon size={20} className="shrink-0 text-brand-agora" aria-hidden="true" />
                    <span>{label}</span>
                  </button>
                );
              })}
            </nav>

            {/* Bottom: signed-in identity + sign out */}
            <div className="shrink-0 border-t border-border px-4 py-4">
              <div className="flex flex-col gap-3">
                <p className="text-[0.75rem] text-text-mute">
                  Signed in as{' '}
                  <span className="font-medium text-text">{displayUserLabel}</span>
                </p>
                <button
                  type="button"
                  className="flex items-center gap-2 text-[0.875rem] text-text-mute transition-colors hover:text-text"
                  onClick={toggleDark}
                >
                  {isDark ? <Sun size={16} aria-hidden="true" /> : <Moon size={16} aria-hidden="true" />}
                  {isDark ? 'Light mode' : 'Dark mode'}
                </button>
                <button
                  type="button"
                  className="flex items-center gap-2 text-[0.875rem] text-text-mute transition-colors hover:text-danger disabled:opacity-50"
                  onClick={() => {
                    setDrawerOpen(false);
                    void handleLogout();
                  }}
                  disabled={loggingOut}
                >
                  <LogOut size={16} aria-hidden="true" />
                  {loggingOut ? 'Signing out…' : 'Sign out'}
                </button>
              </div>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function initialsForEmail(email: string): string {
  const localPart = email.split('@')[0] || email;
  const initials = localPart
    .split('.')
    .filter(Boolean)
    .map((segment) => segment[0]?.toUpperCase())
    .join('');

  return initials || email[0]?.toUpperCase() || '--';
}

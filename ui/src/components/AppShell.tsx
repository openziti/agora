import type { ReactNode } from 'react';

import { BrandMark, type Product } from './BrandMark';
import { NavTabs, type NavTab } from './NavTabs';
import { OrgIndicator } from './OrgIndicator';
import { StatusPill, type StatusPillStatus } from './StatusPill';
import { UserBadge } from './UserBadge';

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

export function AppShell({
  product = 'agora',
  organizationName,
  activeTab,
  navItems,
  status = 'success',
  statusLabel = 'All systems operational',
  userInitials,
  userLabel,
  onTabChange,
  fullHeight = true,
  children,
}: AppShellProps) {
  return (
    <div className={`${fullHeight ? 'min-h-screen' : 'min-h-[28rem]'} bg-page text-text`}>
      <header className="border-b border-border bg-panel">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-8 py-4">
          <div className="flex min-w-0 flex-wrap items-center gap-4">
            <BrandMark product={product} />
            <OrgIndicator organizationName={organizationName} />
          </div>
          <div className="flex min-w-0 items-center gap-3">
            <StatusPill status={status} label={statusLabel} className="max-w-60" />
            <UserBadge initials={userInitials} label={userLabel} />
          </div>
        </div>
        <div className="mx-auto max-w-7xl px-8">
          <NavTabs product={product} items={navItems} activeId={activeTab} onTabChange={onTabChange} />
        </div>
      </header>
      <main className="mx-auto w-full max-w-7xl px-8 py-6">{children}</main>
    </div>
  );
}

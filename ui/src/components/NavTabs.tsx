import { ArrowLeftRight, BookOpen, FileCheck2, LayoutDashboard, ScrollText, ShieldCheck, type LucideIcon } from 'lucide-react';

import type { Product } from './BrandMark';

export type NavTab = {
  id: string;
  label: string;
  icon?: LucideIcon;
};

export type NavTabsProps = {
  product?: Product;
  items?: NavTab[];
  activeId: string;
  onTabChange?: (id: string) => void;
  collapsed?: boolean;
  className?: string;
};

const defaultNavTabs: NavTab[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'sessions', label: 'Sessions' },
  { id: 'workgroups', label: 'Workgroups' },
  { id: 'catalog', label: 'Catalog' },
  { id: 'contracts', label: 'Contracts' },
  { id: 'audit', label: 'Audit' },
];

const defaultIconMap: Record<string, LucideIcon> = {
  dashboard: LayoutDashboard,
  sessions: ArrowLeftRight,
  workgroups: ShieldCheck,
  catalog: BookOpen,
  contracts: FileCheck2,
  audit: ScrollText,
};

export function NavTabs({ items = defaultNavTabs, activeId, onTabChange, collapsed = false, className }: NavTabsProps) {
  return (
    <nav
      className={['flex flex-col gap-0.5', className].filter(Boolean).join(' ')}
      aria-label="Primary navigation"
    >
      {items.map((item) => {
        const isActive = item.id === activeId;
        const Icon = item.icon ?? defaultIconMap[item.id];

        return (
          <button
            key={item.id}
            type="button"
            className={[
              'flex w-full items-center gap-3 rounded-[5px] px-3 py-2 text-[0.875rem] transition-colors',
              'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
              collapsed ? 'justify-center' : '',
              isActive
                ? 'bg-panel-subtle font-bold text-text'
                : 'font-normal text-text-mute hover:bg-panel-subtle',
            ]
              .filter(Boolean)
              .join(' ')}
            aria-current={isActive ? 'page' : undefined}
            title={collapsed ? item.label : undefined}
            onClick={() => onTabChange?.(item.id)}
          >
            {Icon && <Icon size={20} className="shrink-0 text-brand-agora" aria-hidden="true" />}
            {!collapsed && <span>{item.label}</span>}
          </button>
        );
      })}
    </nav>
  );
}

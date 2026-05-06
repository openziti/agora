import type { Product } from './BrandMark';

export type NavTab = {
  id: string;
  label: string;
};

const defaultNavTabs: NavTab[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'sessions', label: 'Sessions' },
  { id: 'workgroups', label: 'Workgroups' },
  { id: 'catalog', label: 'Catalog' },
  { id: 'contracts', label: 'Contracts' },
  { id: 'audit', label: 'Audit' },
];

const navActiveClassNames: Record<Product, string> = {
  agora: 'border-brand-agora bg-brand-agora/10 text-brand-agora',
  llm: 'border-brand-llm bg-brand-llm/10 text-brand-llm',
  mcp: 'border-brand-mcp bg-brand-mcp/10 text-brand-mcp',
};

const navFocusClassNames: Record<Product, string> = {
  agora: 'focus-visible:outline-brand-agora',
  llm: 'focus-visible:outline-brand-llm',
  mcp: 'focus-visible:outline-brand-mcp',
};

export type NavTabsProps = {
  product: Product;
  items?: NavTab[];
  activeId: string;
  onTabChange?: (id: string) => void;
  className?: string;
};

export function NavTabs({ product, items = defaultNavTabs, activeId, onTabChange, className }: NavTabsProps) {
  return (
    <nav
      className={['flex min-w-0 gap-1 overflow-x-auto border-t border-border py-2', className]
        .filter(Boolean)
        .join(' ')}
      aria-label="Primary navigation"
    >
      {items.map((item) => {
        const isActive = item.id === activeId;

        return (
          <button
            key={item.id}
            type="button"
            className={[
              'h-9 shrink-0 rounded-pill border px-3 text-body font-medium focus-visible:outline-2 focus-visible:outline-offset-2',
              isActive
                ? navActiveClassNames[product]
                : 'border-transparent text-text-mute hover:border-border hover:bg-panel-subtle hover:text-text-mute-strong',
              navFocusClassNames[product],
            ].join(' ')}
            aria-current={isActive ? 'page' : undefined}
            onClick={() => onTabChange?.(item.id)}
          >
            {item.label}
          </button>
        );
      })}
    </nav>
  );
}

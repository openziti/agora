import type { SurfaceAccent } from './StatCard';

export type SidebarBreakdownItem = {
  label: string;
  value: number | string;
  barValue?: number;
  accent?: SurfaceAccent;
  tooltip?: string;
};

export type SidebarBreakdownProps = {
  items: SidebarBreakdownItem[];
  className?: string;
};

const barClassNames: Record<SurfaceAccent, string> = {
  agora: 'bg-brand-agora',
  llm: 'bg-brand-llm',
  mcp: 'bg-brand-mcp',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
  neutral: 'bg-text-mute-2',
};

export function SidebarBreakdown({ items, className }: SidebarBreakdownProps) {
  const max = Math.max(
    ...items.map((item) => (typeof item.value === 'number' ? item.value : item.barValue ?? 0)),
    1,
  );

  return (
    <div className={['flex flex-col gap-4', className].filter(Boolean).join(' ')}>
      {items.map((item) => {
        const numericValue = typeof item.value === 'number' ? item.value : item.barValue ?? 0;
        const width = `${Math.max((numericValue / max) * 100, numericValue > 0 ? 4 : 0)}%`;
        const accent = item.accent ?? 'agora';

        return (
          <div key={item.label} className="space-y-2">
            <div className="flex items-center justify-between gap-4">
              <p className="min-w-0 truncate text-body font-medium text-text" title={item.tooltip}>{item.label}</p>
              <p className="shrink-0 text-table text-text-mute-strong">{item.value}</p>
            </div>
            <div className="h-2 overflow-hidden rounded-status bg-panel-subtle">
              <div className={`h-full rounded-status ${barClassNames[accent]}`} style={{ width }} />
            </div>
          </div>
        );
      })}
    </div>
  );
}

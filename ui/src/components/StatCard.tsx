import { ArrowDownRight, ArrowRight, ArrowUpRight, type LucideIcon } from 'lucide-react';

export type SurfaceAccent = 'agora' | 'llm' | 'mcp' | 'success' | 'warning' | 'danger' | 'info' | 'neutral';

export type StatCardDelta = {
  value: string;
  direction: 'up' | 'down' | 'flat';
  label?: string;
};

export type StatCardProps = {
  label: string;
  value: string | number;
  icon?: LucideIcon;
  delta?: StatCardDelta;
  accent?: SurfaceAccent;
  className?: string;
};

const accentClassNames: Record<SurfaceAccent, string> = {
  agora: 'text-brand-agora bg-brand-agora/10',
  llm: 'text-brand-llm bg-brand-llm/10',
  mcp: 'text-brand-mcp bg-brand-mcp/10',
  success: 'text-success-strong bg-success/10',
  warning: 'text-warning bg-warning/10',
  danger: 'text-danger bg-danger/10',
  info: 'text-info bg-info/10',
  neutral: 'text-text-mute bg-panel-subtle',
};

const deltaClassNames: Record<StatCardDelta['direction'], string> = {
  up: 'text-success-strong',
  down: 'text-danger',
  flat: 'text-text-mute',
};

const deltaIcons: Record<StatCardDelta['direction'], LucideIcon> = {
  up: ArrowUpRight,
  down: ArrowDownRight,
  flat: ArrowRight,
};

export function StatCard({ label, value, icon: Icon, delta, accent = 'agora', className }: StatCardProps) {
  const DeltaIcon = delta ? deltaIcons[delta.direction] : undefined;

  return (
    <section
      className={[
        'rounded-card border border-border bg-panel p-4',
        'flex min-h-32 flex-col justify-between gap-4',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="flex items-start justify-between gap-3">
        <p className="text-label font-medium uppercase text-text-mute">{label}</p>
        {Icon ? (
          <div className={`flex size-9 shrink-0 items-center justify-center rounded-pill ${accentClassNames[accent]}`}>
            <Icon size={18} aria-hidden="true" />
          </div>
        ) : null}
      </div>
      <div>
        <p className="text-stat font-bold text-text">{value}</p>
        {delta && DeltaIcon ? (
          <p className={`mt-2 flex items-center gap-1 text-table font-medium ${deltaClassNames[delta.direction]}`}>
            <DeltaIcon size={15} aria-hidden="true" />
            <span>{delta.value}</span>
            {delta.label ? <span className="text-text-mute">{delta.label}</span> : null}
          </p>
        ) : null}
      </div>
    </section>
  );
}

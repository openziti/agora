import { ArrowDownRight, ArrowRight, ArrowUpRight, ChevronRight, type LucideIcon } from 'lucide-react';

import { InfoTooltip } from './InfoTooltip';

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
  tooltip?: string;
  className?: string;
  onClick?: () => void;
  caret?: boolean;
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

export function StatCard({ label, value, icon: Icon, delta, accent = 'agora', tooltip, className, onClick, caret }: StatCardProps) {
  const DeltaIcon = delta ? deltaIcons[delta.direction] : undefined;

  return (
    <section
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      className={[
        'rounded-[7px] border border-border bg-panel p-[1.05rem]',
        'flex flex-col justify-between gap-2',
        onClick ? 'cursor-pointer hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora' : '',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="flex items-start justify-between gap-3">
        <p className="flex items-center gap-1 text-label font-medium uppercase text-text-mute">
          {label}
          {tooltip ? <InfoTooltip content={tooltip} ariaLabel={tooltip} /> : null}
        </p>
        {Icon ? (
          <div className={`flex size-8 shrink-0 items-center justify-center rounded-pill ${accentClassNames[accent]}`}>
            <Icon size={16} aria-hidden="true" />
          </div>
        ) : null}
      </div>
      <div className={caret ? 'flex items-end justify-between gap-2' : undefined}>
        <div>
          <p className="text-[1.5rem] font-bold leading-tight text-text">{value}</p>
          {delta && DeltaIcon ? (
            <p className={`mt-2 flex items-center gap-1 text-table font-medium ${deltaClassNames[delta.direction]}`}>
              <DeltaIcon size={15} aria-hidden="true" />
              <span>{delta.value}</span>
              {delta.label ? <span className="text-text-mute">{delta.label}</span> : null}
            </p>
          ) : null}
        </div>
        {caret ? <ChevronRight size={14} aria-hidden="true" className="shrink-0 text-text-mute" /> : null}
      </div>
    </section>
  );
}

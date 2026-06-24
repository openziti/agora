export type StatusPillStatus =
  | 'success'
  | 'warning'
  | 'danger'
  | 'info'
  | 'neutral'
  | 'online'
  | 'stale'
  | 'unknown'
  | 'disabled'
  | 'active'
  | 'failed';

export type StatusPillProps = {
  status: StatusPillStatus;
  label: string;
  className?: string;
  title?: string;
};

const statusPillClassNames: Record<StatusPillStatus, string> = {
  success: 'bg-success/15 text-success-strong border-success/30',
  online: 'bg-success/15 text-success-strong border-success/30',
  active: 'bg-success/15 text-success-strong border-success/30',
  warning: 'bg-warning/15 text-warning-strong border-warning/30',
  stale: 'bg-warning/15 text-warning-strong border-warning/30',
  danger: 'bg-danger/15 text-danger border-danger/30',
  failed: 'bg-danger/15 text-danger border-danger/30',
  disabled: 'bg-danger/15 text-danger border-danger/30',
  info: 'bg-info/15 text-info border-info/30',
  neutral: 'bg-panel-subtle text-text-mute border-border',
  unknown: 'bg-panel-subtle text-text-mute border-border',
};

export function StatusPill({ status, label, className, title }: StatusPillProps) {
  return (
    <span
      title={title}
      className={[
        'inline-flex h-6 max-w-full items-center rounded-status border px-2.5 text-[0.6875rem] font-semibold',
        statusPillClassNames[status],
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <span className="truncate">{label}</span>
    </span>
  );
}

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
};

const statusDotClassNames: Record<StatusPillStatus, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  info: 'bg-info',
  neutral: 'bg-text-mute-2',
  online: 'bg-success',
  stale: 'bg-warning',
  unknown: 'bg-text-mute-2',
  disabled: 'bg-danger',
  active: 'bg-success',
  failed: 'bg-danger',
};

export function StatusPill({ status, label, className }: StatusPillProps) {
  return (
    <span
      className={[
        'inline-flex h-8 max-w-full items-center gap-2 rounded-status border border-border bg-panel px-3 text-table font-medium text-text-mute-strong',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <span className={`size-2 shrink-0 rounded-status ${statusDotClassNames[status]}`} aria-hidden="true" />
      <span className="truncate">{label}</span>
    </span>
  );
}

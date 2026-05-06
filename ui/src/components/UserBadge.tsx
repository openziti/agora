export type UserBadgeProps = {
  initials?: string;
  label?: string;
  className?: string;
};

export function UserBadge({ initials = 'DU', label = 'Demo user', className }: UserBadgeProps) {
  return (
    <div
      className={[
        'flex size-9 shrink-0 items-center justify-center rounded-status border border-border bg-panel-subtle text-table font-semibold text-text-mute-strong',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      aria-label={label}
      title={label}
    >
      {initials}
    </div>
  );
}

import { Inbox, type LucideIcon } from 'lucide-react';

export type EmptyStateProps = {
  title: string;
  description?: string;
  icon?: LucideIcon;
  className?: string;
};

export function EmptyState({ title, description, icon: Icon = Inbox, className }: EmptyStateProps) {
  return (
    <div
      className={[
        'flex min-h-40 flex-col items-center justify-center rounded-card border border-dashed border-border bg-panel-subtle px-6 py-8 text-center',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div className="mb-3 flex size-10 items-center justify-center rounded-status bg-panel text-text-mute">
        <Icon size={20} aria-hidden="true" />
      </div>
      <h3 className="text-body font-semibold text-text">{title}</h3>
      {description ? <p className="mt-1 max-w-md text-body text-text-mute">{description}</p> : null}
    </div>
  );
}

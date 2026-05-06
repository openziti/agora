import { ChevronDown, Globe } from 'lucide-react';

export type OrgIndicatorProps = {
  organizationName: string;
  className?: string;
};

export function OrgIndicator({ organizationName, className }: OrgIndicatorProps) {
  return (
    <div
      className={[
        'flex h-9 min-w-0 max-w-72 items-center gap-2 rounded-pill border border-border bg-panel-subtle px-3 text-body text-text-mute-strong',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
      aria-label={`Organization: ${organizationName}`}
    >
      <Globe size={16} aria-hidden="true" className="shrink-0 text-text-mute" />
      <span className="truncate">{organizationName}</span>
      <ChevronDown size={15} aria-hidden="true" className="shrink-0 text-text-mute-2" />
    </div>
  );
}

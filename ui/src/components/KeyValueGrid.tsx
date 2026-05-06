import type { ReactNode } from 'react';

export type KeyValueEntry = {
  key: string;
  value: ReactNode;
};

export type KeyValueGridProps = {
  entries: KeyValueEntry[];
  className?: string;
};

export function KeyValueGrid({ entries, className }: KeyValueGridProps) {
  return (
    <dl className={['grid gap-3 sm:grid-cols-2', className].filter(Boolean).join(' ')}>
      {entries.map((entry) => (
        <div key={entry.key} className="rounded-card border border-border bg-panel-subtle p-3">
          <dt className="text-label font-medium uppercase text-text-mute">{entry.key}</dt>
          <dd className="mt-1 break-words text-body font-medium text-text">{entry.value}</dd>
        </div>
      ))}
    </dl>
  );
}

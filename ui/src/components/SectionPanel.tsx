import type { ReactNode } from 'react';

export type SectionPanelProps = {
  title?: string;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
};

export function SectionPanel({ title, actions, children, className, bodyClassName }: SectionPanelProps) {
  const hasHeader = Boolean(title || actions);

  return (
    <section className={['rounded-card border border-border bg-panel', className].filter(Boolean).join(' ')}>
      {hasHeader ? (
        <header className="flex min-h-14 items-center justify-between gap-4 border-b border-border px-5 py-3">
          {title ? <h2 className="text-section font-semibold text-text">{title}</h2> : <div />}
          {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
        </header>
      ) : null}
      <div className={['p-5', bodyClassName].filter(Boolean).join(' ')}>{children}</div>
    </section>
  );
}

import type { LucideIcon } from 'lucide-react';
import { Info } from 'lucide-react';

export type PageHeaderProps = {
  icon: LucideIcon;
  label: string;
  title: string;
  description: string;
  onInfoClick: () => void;
};

export function PageHeader({ icon: Icon, label, title, description, onInfoClick }: PageHeaderProps) {
  return (
    <section className="rounded-card border border-border bg-panel p-4">
      <div className="flex items-start gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-[7px] bg-brand-agora/10 text-brand-agora">
          <Icon size={18} aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] text-text-mute-strong">
            {label}
          </p>
          <h1 className="mt-0.5 text-xl font-bold text-text">{title}</h1>
          <p className="mt-1 text-body leading-relaxed text-text-mute">{description}</p>
        </div>
        <button
          type="button"
          className="mt-0.5 flex shrink-0 items-center gap-1 text-[0.76rem] text-text-mute-2 transition-colors hover:text-brand-agora focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          onClick={onInfoClick}
          aria-label={`Learn more about ${title}`}
        >
          <Info size={13} aria-hidden="true" />
          <span>Learn more</span>
        </button>
      </div>
    </section>
  );
}

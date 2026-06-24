import { type ReactNode } from 'react';
import { Info, type LucideIcon } from 'lucide-react';

export function DrawerDivider() {
  return <hr className="border-border" />;
}

export type DrawerCardProps = {
  icon: LucideIcon;
  title: string;
  description: ReactNode;
};

export function DrawerCard({ icon: Icon, title, description }: DrawerCardProps) {
  return (
    <div className="flex gap-3 rounded-[7px] border border-border bg-panel p-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-[5px] bg-brand-agora/10 text-brand-agora">
        <Icon size={16} aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <p className="text-[0.8125rem] font-semibold text-text">{title}</p>
        <div className="mt-0.5 text-[0.76rem] leading-relaxed text-text-mute">{description}</div>
      </div>
    </div>
  );
}

export type DrawerCodeChipProps = {
  children: ReactNode;
};

export function DrawerCodeChip({ children }: DrawerCodeChipProps) {
  return (
    <code className="rounded-[4px] border border-border bg-panel-subtle px-1.5 py-0.5 font-mono text-[0.6875rem] text-text-mute-strong">
      {children}
    </code>
  );
}

export type DrawerTipProps = {
  children: ReactNode;
};

export function DrawerTip({ children }: DrawerTipProps) {
  return (
    <div className="flex gap-3 rounded-[7px] border border-info/30 bg-info/5 p-3">
      <Info size={15} className="mt-0.5 shrink-0 text-info" aria-hidden="true" />
      <p className="text-[0.76rem] leading-relaxed text-text-mute">{children}</p>
    </div>
  );
}

export type DrawerStep = {
  name: string;
  description: string;
};

export type DrawerStepListProps = {
  steps: DrawerStep[];
};

export function DrawerStepList({ steps }: DrawerStepListProps) {
  return (
    <div className="flex flex-col">
      {steps.map((step, i) => (
        <div key={step.name} className="flex gap-3">
          <div className="flex flex-col items-center">
            <div className="flex size-6 shrink-0 items-center justify-center rounded-full border-2 border-brand-agora bg-brand-agora/10 text-[0.625rem] font-bold text-brand-agora">
              {i + 1}
            </div>
            {i < steps.length - 1 ? (
              <div className="my-1 w-px flex-1 bg-border" />
            ) : null}
          </div>
          <div className="min-w-0 pb-3">
            <p className="text-[0.8125rem] font-semibold text-text">{step.name}</p>
            <p className="mt-0.5 text-[0.76rem] leading-relaxed text-text-mute">{step.description}</p>
          </div>
        </div>
      ))}
    </div>
  );
}

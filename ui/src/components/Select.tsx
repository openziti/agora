import type { SelectHTMLAttributes } from 'react';

export type SelectProps = SelectHTMLAttributes<HTMLSelectElement> & {
  error?: boolean;
};

export function Select({ error, className, children, ...props }: SelectProps) {
  return (
    <select
      {...props}
      className={[
        'h-9 cursor-pointer appearance-none rounded-[5px] border bg-panel-subtle py-[0.375rem] px-[0.5625rem] text-[0.76rem] text-text-mute-strong transition-colors',
        'focus:outline-none focus:ring-2 focus:ring-brand-agora focus:border-brand-agora',
        error ? 'border-danger' : 'border-border',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {children}
    </select>
  );
}

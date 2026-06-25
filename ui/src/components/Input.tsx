import type { InputHTMLAttributes } from 'react';

export type InputProps = InputHTMLAttributes<HTMLInputElement> & {
  error?: boolean;
};

export function Input({ error, className, ...props }: InputProps) {
  return (
    <input
      {...props}
      className={[
        'h-9 w-full rounded-[5px] border bg-panel-subtle px-[0.5625rem] py-[0.375rem] text-[0.76rem] text-text-mute-strong transition-colors',
        'placeholder:text-text-mute-2',
        'focus:outline-none focus:ring-2 focus:ring-brand-agora focus:border-brand-agora',
        error ? 'border-danger' : 'border-border',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    />
  );
}

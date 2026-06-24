import type { ButtonHTMLAttributes } from 'react';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost';

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
};

const variantClassNames: Record<ButtonVariant, string> = {
  primary:   'px-[0.875rem] py-[0.4375rem] bg-brand-agora text-white border-transparent hover:bg-brand-agora-hover',
  secondary: 'px-[0.875rem] py-[0.4375rem] bg-panel-subtle text-text-mute-strong border-border hover:bg-border-light',
  ghost:     'px-2 py-1 bg-transparent text-text-mute border-transparent hover:bg-panel-subtle hover:text-text-mute-strong',
};

export function Button({ variant = 'primary', className, children, ...props }: ButtonProps) {
  return (
    <button
      {...props}
      className={[
        'inline-flex cursor-pointer items-center justify-center rounded-[5px] border text-[0.76rem] font-medium transition-colors',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        'disabled:cursor-not-allowed disabled:opacity-50',
        variantClassNames[variant],
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {children}
    </button>
  );
}

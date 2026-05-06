import { Cpu, Layers, ShieldCheck, type LucideIcon } from 'lucide-react';

export type Product = 'agora' | 'llm' | 'mcp';

type ProductTheme = {
  name: string;
  icon: LucideIcon;
  tileClassName: string;
  textClassName: string;
};

const productThemes: Record<Product, ProductTheme> = {
  agora: {
    name: 'Agora',
    icon: ShieldCheck,
    tileClassName: 'bg-[linear-gradient(135deg,var(--color-brand-agora),var(--color-brand-agora-end))]',
    textClassName: 'text-brand-agora',
  },
  llm: {
    name: 'LLM Gateway',
    icon: Cpu,
    tileClassName: 'bg-[linear-gradient(135deg,var(--color-brand-llm),var(--color-brand-llm-end))]',
    textClassName: 'text-brand-llm',
  },
  mcp: {
    name: 'MCP Gateway',
    icon: Layers,
    tileClassName: 'bg-[linear-gradient(135deg,var(--color-brand-mcp),var(--color-brand-mcp-end))]',
    textClassName: 'text-brand-mcp',
  },
};

export type BrandMarkProps = {
  product: Product;
  className?: string;
};

export function BrandMark({ product, className }: BrandMarkProps) {
  const theme = productThemes[product];
  const Icon = theme.icon;

  return (
    <div className={['flex min-w-0 items-center gap-3', className].filter(Boolean).join(' ')}>
      <div className={`flex size-11 shrink-0 items-center justify-center rounded-card text-white ${theme.tileClassName}`}>
        <Icon size={23} strokeWidth={2.25} aria-hidden="true" />
      </div>
      <div className="min-w-0">
        <p className={`truncate text-section font-semibold ${theme.textClassName}`}>{theme.name}</p>
        <p className="truncate text-[0.6875rem] leading-4 text-text-mute">by NetFoundry</p>
      </div>
    </div>
  );
}

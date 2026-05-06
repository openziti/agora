type ColorToken = {
  name: string;
  className: string;
  hex: string;
  usage: string;
};

const colors: ColorToken[] = [
  { name: 'page', className: 'bg-page', hex: '#fafafa', usage: 'page background' },
  { name: 'panel', className: 'bg-panel', hex: '#ffffff', usage: 'card and panel surfaces' },
  { name: 'panel-subtle', className: 'bg-panel-subtle', hex: '#f4f4f5', usage: 'nested panels' },
  { name: 'border', className: 'bg-border', hex: '#e4e4e7', usage: 'card borders' },
  { name: 'border-strong', className: 'bg-border-strong', hex: '#d4d4d8', usage: 'separators' },
  { name: 'text-mute-2', className: 'bg-text-mute-2', hex: '#a1a1aa', usage: 'tertiary text' },
  { name: 'text-mute', className: 'bg-text-mute', hex: '#71717a', usage: 'secondary text' },
  { name: 'text-mute-strong', className: 'bg-text-mute-strong', hex: '#52525b', usage: 'headers' },
  { name: 'text', className: 'bg-text', hex: '#18181b', usage: 'primary text' },
  { name: 'brand-llm', className: 'bg-brand-llm', hex: '#0891b2', usage: 'LLM Gateway' },
  { name: 'brand-llm-end', className: 'bg-brand-llm-end', hex: '#0284c7', usage: 'LLM gradient end' },
  { name: 'brand-mcp', className: 'bg-brand-mcp', hex: '#7c3aed', usage: 'MCP Gateway' },
  { name: 'brand-mcp-end', className: 'bg-brand-mcp-end', hex: '#a855f7', usage: 'MCP gradient end' },
  { name: 'brand-agora', className: 'bg-brand-agora', hex: '#4f46e5', usage: 'Agora' },
  { name: 'brand-agora-end', className: 'bg-brand-agora-end', hex: '#1d4ed8', usage: 'Agora gradient end' },
  { name: 'brand-family-start', className: 'bg-brand-family-start', hex: '#0891b2', usage: 'family gradient start' },
  { name: 'brand-family-end', className: 'bg-brand-family-end', hex: '#7c3aed', usage: 'family gradient end' },
  { name: 'success', className: 'bg-success', hex: '#22c55e', usage: 'healthy' },
  { name: 'success-strong', className: 'bg-success-strong', hex: '#16a34a', usage: 'active' },
  { name: 'warning', className: 'bg-warning', hex: '#d97706', usage: 'warning' },
  { name: 'warning-strong', className: 'bg-warning-strong', hex: '#f97316', usage: 'degraded' },
  { name: 'danger', className: 'bg-danger', hex: '#dc2626', usage: 'failed' },
  { name: 'info', className: 'bg-info', hex: '#2563eb', usage: 'info' },
  { name: 'highlight', className: 'bg-highlight', hex: '#eab308', usage: 'highlight' },
];

const typeRows = [
  {
    label: 'Stat',
    className: 'text-stat font-bold text-text',
    sample: '1,284',
    detail: '28-32px / weight 700',
  },
  {
    label: 'Section',
    className: 'text-section font-semibold text-text',
    sample: 'Active Sessions',
    detail: '18-20px / weight 600',
  },
  {
    label: 'Body',
    className: 'text-body text-text',
    sample: 'Governed collaboration activity across organizations.',
    detail: '14px / weight 400',
  },
  {
    label: 'Label',
    className: 'text-label font-medium uppercase text-text-mute',
    sample: 'close reason',
    detail: '11-12px / weight 500',
  },
  {
    label: 'Table',
    className: 'text-table text-text',
    sample: 'con_k9x2e8p31wqz',
    detail: '13px / weight 400',
  },
];

export default function Kit() {
  return (
    <main className="min-h-screen bg-page px-8 py-10 text-text">
      <section className="mx-auto flex max-w-6xl flex-col gap-8">
        <header>
          <p className="text-label font-medium uppercase text-text-mute">Agora UI kit</p>
          <h1 className="mt-2 text-2xl font-semibold text-text">Design Tokens</h1>
        </header>

        <section className="rounded-card border border-border bg-panel p-6">
          <h2 className="mb-4 text-section font-semibold text-text">Type</h2>
          <div className="divide-y divide-border">
            {typeRows.map((row) => (
              <div key={row.label} className="grid gap-3 py-4 md:grid-cols-[160px_1fr_220px]">
                <p className="text-label font-medium uppercase text-text-mute">{row.label}</p>
                <p className={row.className}>{row.sample}</p>
                <p className="text-table text-text-mute">{row.detail}</p>
              </div>
            ))}
          </div>
        </section>

        <section className="rounded-card border border-border bg-panel p-6">
          <h2 className="mb-4 text-section font-semibold text-text">Colors</h2>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
            {colors.map((color) => (
              <div key={color.name} className="flex items-center gap-3 rounded-card border border-border bg-panel p-3">
                <div className={`size-10 rounded-pill border border-border-strong ${color.className}`} />
                <div>
                  <p className="font-mono text-table text-text">{color.name}</p>
                  <p className="text-table text-text-mute">
                    {color.hex} - {color.usage}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </section>
      </section>
    </main>
  );
}

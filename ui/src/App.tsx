import { Activity, Boxes, ShieldCheck } from 'lucide-react';

export function App() {
  return (
    <main className="min-h-screen bg-page px-8 py-10 text-text">
      <section className="mx-auto flex max-w-5xl flex-col gap-8">
        <header className="flex items-center justify-between border-b border-border pb-6">
          <div className="flex items-center gap-4">
            <div className="flex size-12 items-center justify-center rounded-card bg-brand-agora text-white">
              <ShieldCheck size={26} strokeWidth={2.25} aria-hidden="true" />
            </div>
            <div>
              <p className="text-xs font-medium uppercase text-text-mute">NetFoundry</p>
              <h1 className="text-2xl font-semibold text-text">Agora Dashboard</h1>
            </div>
          </div>
          <div className="rounded-pill border border-border bg-panel px-3 py-1.5 text-sm font-medium text-text-mute-strong">
            UI foundation
          </div>
        </header>

        <div className="grid gap-4 md:grid-cols-3">
          <div className="rounded-card border border-border bg-panel p-5">
            <div className="mb-4 flex items-center gap-3 text-brand-agora">
              <Activity size={20} aria-hidden="true" />
              <h2 className="text-lg font-semibold text-text">Live Activity</h2>
            </div>
            <p className="text-sm leading-6 text-text-mute">
              Tailwind v4 is reading the Agora design tokens from the CSS-first theme.
            </p>
          </div>
          <div className="rounded-card border border-border bg-panel p-5">
            <div className="mb-4 flex items-center gap-3 text-info">
              <Boxes size={20} aria-hidden="true" />
              <h2 className="text-lg font-semibold text-text">Local Assets</h2>
            </div>
            <p className="text-sm leading-6 text-text-mute">
              Inter and JetBrains Mono are bundled with the app for offline demo rooms.
            </p>
          </div>
          <div className="rounded-card border border-border bg-panel-subtle p-5">
            <p className="mb-3 font-mono text-xs text-text-mute-strong">res_agora_ui</p>
            <p className="text-sm leading-6 text-text-mute">
              This placeholder will be replaced by the dashboard screens in Track C.
            </p>
          </div>
        </div>
      </section>
    </main>
  );
}

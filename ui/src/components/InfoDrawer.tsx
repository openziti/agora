import { useEffect, type ReactNode } from 'react';
import { GripVertical, X } from 'lucide-react';

import { useResizableDrawer } from '../hooks/useResizableDrawer';

export type InfoDrawerProps = {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
};

export function InfoDrawer({ title, onClose, children, wide = false }: InfoDrawerProps) {
  const { width, dragHandleProps } = useResizableDrawer({
    defaultWidth: wide ? 520 : 400,
    minWidth: 320,
    maxWidth: 800,
  });

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  return (
    <>
      <div className="fixed inset-0 z-[199] bg-text/20" aria-hidden="true" onClick={onClose} />
      <aside
        style={{ width }}
        className="fixed right-0 top-0 z-[200] flex h-full flex-col border-l border-border bg-page shadow-xl"
        role="dialog"
        aria-modal="true"
        aria-labelledby="info-drawer-title"
      >
        <div
          {...dragHandleProps}
          aria-hidden="true"
          className="absolute left-0 top-1/2 z-10 flex h-9 w-5 -translate-x-full -translate-y-1/2 cursor-col-resize items-center justify-center rounded-md border border-border bg-page shadow-md text-text-mute transition-colors hover:bg-panel-subtle hover:text-text"
        >
          <GripVertical size={14} aria-hidden="true" />
        </div>
        <header className="flex min-h-14 shrink-0 items-center justify-between gap-4 border-b border-border bg-panel px-5 py-3">
          <h2 id="info-drawer-title" className="text-[0.875rem] font-semibold text-text">
            {title}
          </h2>
          <button
            type="button"
            className="inline-flex size-8 shrink-0 items-center justify-center rounded-[5px] border border-border bg-panel text-text-mute-strong hover:bg-panel-subtle focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            aria-label="Close"
            onClick={onClose}
          >
            <X size={15} aria-hidden="true" />
          </button>
        </header>
        <div className="flex-1 overflow-y-auto p-5 text-[0.76rem]">
          {children}
        </div>
      </aside>
    </>
  );
}

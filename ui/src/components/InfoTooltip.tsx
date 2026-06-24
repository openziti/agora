import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { Info } from 'lucide-react';

/*
 * InfoTooltip — a shared click-to-open info tooltip.
 *
 * A small lucide Info icon (size 12, text-text-mute) acts as the trigger button.
 * Clicking it toggles a floating panel open/closed; clicking the icon again,
 * clicking anywhere outside, or pressing Escape closes it. There is no hover
 * behavior, no native `title` attribute, and no open/close delay. The cursor is
 * a pointer on the icon (never cursor-help).
 *
 * The panel is positioned manually relative to the trigger using
 * getBoundingClientRect (same approach as ViolationTooltip in AuditLog.tsx) and
 * is flipped/shifted to avoid overflowing the viewport. All rect reads are
 * guarded against null refs so the component never crashes if the trigger
 * unmounts mid-measure.
 *
 * Props:
 *   - content:   string | ReactNode (required) — tooltip body; ReactNode allows
 *                light formatting.
 *   - ariaLabel: string (optional) — accessible label for the trigger button.
 *                Defaults to 'More information'.
 *   - side:      'top' | 'bottom' (optional) — preferred placement hint. The
 *                component still flips to the opposite side when there is not
 *                enough room. Defaults to 'top'.
 *   - className: string (optional) — extra classes for the trigger button.
 *
 * Usage:
 *   <InfoTooltip content="Active advertisements published by your organization." />
 *   <InfoTooltip content={<>Supports <code>http</code> and <code>tcp</code>.</>} ariaLabel="About protocols" />
 *
 * This component is intentionally self-contained. Wiring it into specific call
 * sites (Dashboard advertisements icon, Audit Violations card, etc.) is handled
 * in follow-up changes.
 */

export type InfoTooltipSide = 'top' | 'bottom';

export type InfoTooltipProps = {
  content: string | ReactNode;
  ariaLabel?: string;
  side?: InfoTooltipSide;
  className?: string;
};

const PANEL_WIDTH = 260;
const GAP = 6;
const MARGIN = 8;

type PanelPosition = {
  top?: number;
  bottom?: number;
  left: number;
  caretOffset: number;
  renderBelow: boolean;
};

export function InfoTooltip({ content, ariaLabel = 'More information', side = 'top', className }: InfoTooltipProps) {
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState<PanelPosition | null>(null);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);

  // measure and place the panel relative to the trigger, guarding null refs
  useLayoutEffect(() => {
    if (!open) return;

    function place() {
      const rect = triggerRef.current?.getBoundingClientRect();
      if (!rect) return;

      const spaceAbove = rect.top;
      // prefer the requested side, but flip when there isn't room
      const renderBelow = side === 'bottom' ? spaceAbove < 130 : spaceAbove - GAP < 130;

      const top = renderBelow ? rect.bottom + GAP : undefined;
      const bottom = renderBelow ? undefined : window.innerHeight - rect.top + GAP;

      const triggerCenterX = rect.left + rect.width / 2;
      let left = triggerCenterX - PANEL_WIDTH / 2;
      left = Math.max(MARGIN, Math.min(left, window.innerWidth - PANEL_WIDTH - MARGIN));
      const caretOffset = Math.max(8, Math.min(triggerCenterX - left, PANEL_WIDTH - 8));

      setPosition({ top, bottom, left, caretOffset, renderBelow });
    }

    place();
    window.addEventListener('resize', place);
    window.addEventListener('scroll', place, true);
    return () => {
      window.removeEventListener('resize', place);
      window.removeEventListener('scroll', place, true);
    };
  }, [open, side]);

  // outside-click closes the panel
  useEffect(() => {
    if (!open) return;
    function handleOutside(e: MouseEvent) {
      const target = e.target as Node;
      if (panelRef.current?.contains(target)) return;
      if (triggerRef.current?.contains(target)) return;
      setOpen(false);
    }
    document.addEventListener('mousedown', handleOutside);
    return () => document.removeEventListener('mousedown', handleOutside);
  }, [open]);

  // Escape closes the panel
  useEffect(() => {
    if (!open) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false);
    }
    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [open]);

  return (
    <>
      <button
        ref={triggerRef}
        type="button"
        aria-label={ariaLabel}
        aria-expanded={open}
        onClick={(e) => {
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        className={[
          'inline-flex shrink-0 cursor-pointer items-center justify-center rounded-pill text-text-mute',
          'transition-colors hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
          className,
        ]
          .filter(Boolean)
          .join(' ')}
      >
        <Info size={12} aria-hidden="true" />
      </button>
      {open && position ? (
        <div
          ref={panelRef}
          role="tooltip"
          className="fixed z-[500] rounded-card border border-border bg-panel p-3 text-table normal-case tracking-normal text-text-mute shadow-xl"
          style={{ top: position.top, bottom: position.bottom, left: position.left, width: PANEL_WIDTH }}
        >
          {position.renderBelow ? (
            <>
              <div style={{ position: 'absolute', top: -8, left: position.caretOffset - 8, width: 0, height: 0, borderStyle: 'solid', borderWidth: '0 8px 8px 8px', borderColor: 'transparent transparent var(--color-border) transparent' }} />
              <div style={{ position: 'absolute', top: -7, left: position.caretOffset - 7, width: 0, height: 0, borderStyle: 'solid', borderWidth: '0 7px 7px 7px', borderColor: 'transparent transparent var(--color-panel) transparent' }} />
            </>
          ) : (
            <>
              <div style={{ position: 'absolute', bottom: -8, left: position.caretOffset - 8, width: 0, height: 0, borderStyle: 'solid', borderWidth: '8px 8px 0 8px', borderColor: 'var(--color-border) transparent transparent transparent' }} />
              <div style={{ position: 'absolute', bottom: -7, left: position.caretOffset - 7, width: 0, height: 0, borderStyle: 'solid', borderWidth: '7px 7px 0 7px', borderColor: 'var(--color-panel) transparent transparent transparent' }} />
            </>
          )}
          {content}
        </div>
      ) : null}
    </>
  );
}

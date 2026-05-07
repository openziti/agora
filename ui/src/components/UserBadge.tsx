import { ChevronDown, LogOut } from 'lucide-react';
import { useEffect, useRef, useState } from 'react';

export type UserBadgeProps = {
  email?: string;
  initials?: string;
  label?: string;
  loggingOut?: boolean;
  onLogout?: () => void;
  className?: string;
};

const avatarClassNames = [
  'bg-brand-agora/10 text-brand-agora',
  'bg-brand-llm/10 text-brand-llm',
  'bg-brand-mcp/10 text-brand-mcp',
  'bg-info/10 text-info',
  'bg-success/10 text-success-strong',
];

export function UserBadge({
  email,
  initials,
  label,
  loggingOut = false,
  onLogout,
  className,
}: UserBadgeProps) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const displayLabel = email ?? label ?? 'Account';
  const displayInitials = email ? initialsFromEmail(email) : initials ?? 'DU';
  const avatarClassName = avatarClassNames[hashString(email ?? displayLabel) % avatarClassNames.length];

  useEffect(() => {
    if (!open) {
      return;
    }

    function handlePointerDown(event: PointerEvent) {
      if (!rootRef.current?.contains(event.target as Node)) {
        setOpen(false);
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        setOpen(false);
      }
    }

    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('keydown', handleKeyDown);
    };
  }, [open]);

  return (
    <div ref={rootRef} className={['relative shrink-0', className].filter(Boolean).join(' ')}>
      <button
        type="button"
        className={[
          'flex size-9 items-center justify-center rounded-status border border-border text-table font-semibold hover:border-border-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
          avatarClassName,
        ].join(' ')}
        aria-label={displayLabel}
        aria-expanded={open}
        aria-haspopup="menu"
        title={displayLabel}
        onClick={() => setOpen((current) => !current)}
      >
        {displayInitials}
      </button>

      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-11 z-30 w-64 rounded-card border border-border bg-panel p-2 shadow-lg"
        >
          <div className="min-w-0 px-3 py-2">
            <p className="truncate text-label font-medium uppercase text-text-mute">Signed in as</p>
            <p className="mt-1 truncate text-body font-medium text-text">{displayLabel}</p>
          </div>
          <button
            type="button"
            role="menuitem"
            disabled={loggingOut}
            className="mt-1 flex h-9 w-full items-center justify-between rounded-pill px-3 text-table font-medium text-text-mute-strong hover:bg-panel-subtle hover:text-text disabled:cursor-not-allowed disabled:opacity-60 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            onClick={() => {
              setOpen(false);
              onLogout?.();
            }}
          >
            <span className="inline-flex items-center gap-2">
              <LogOut size={15} aria-hidden="true" />
              {loggingOut ? 'Signing out' : 'Logout'}
            </span>
            <ChevronDown size={14} aria-hidden="true" className="-rotate-90 text-text-mute-2" />
          </button>
        </div>
      ) : null}
    </div>
  );
}

function initialsFromEmail(email: string): string {
  const localPart = email.split('@')[0] || email;
  const initials = localPart
    .split('.')
    .filter(Boolean)
    .map((segment) => segment[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return initials || localPart.slice(0, 1).toUpperCase() || 'AG';
}

function hashString(value: string): number {
  return Array.from(value).reduce((hash, char) => (hash * 31 + char.charCodeAt(0)) >>> 0, 0);
}

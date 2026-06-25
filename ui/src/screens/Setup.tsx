import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { useNavigate } from 'react-router';
import {
  ArrowLeftRight,
  BookOpen,
  Building2,
  Check,
  CheckCircle2,
  Clock,
  Copy,
  Eye,
  EyeOff,
  FileCheck2,
  Globe,
  Hash,
  Info,
  Lock,
  MessageSquare,
  Minus,
  Network,
  Plus,
  ShieldCheck,
  Tag,
  X,
  Zap,
} from 'lucide-react';

import { BrandMark } from '../components/BrandMark';
import { Button } from '../components/Button';
import { DrawerCard, DrawerCodeChip, DrawerDivider, DrawerTip } from '../components/DrawerContent';
import { InfoDrawer } from '../components/InfoDrawer';
import { Input } from '../components/Input';
import { Select } from '../components/Select';
import { StatusPill } from '../components/StatusPill';
import {
  ApiError,
  createAdvertisement,
  createContract,
  createWorkgroup,
  getAccountToken,
  listContracts,
  listWorkgroups,
  type Advertisement,
  type AdvertisementInteractionPatternKind,
  type AdvertisementTunnelMode,
  type Contract,
  type ContractAccessMode,
  type Workgroup,
  type WorkgroupScope,
} from '../lib/api';

const TOKEN_MASK = '••••••••••••••••••••••••••••••••';

const STEPS: Array<{ number: number; name: string }> = [
  { number: 1, name: 'Organization and Account' },
  { number: 2, name: 'Enroll Environment' },
  { number: 3, name: 'Create Workgroups' },
  { number: 4, name: 'Publish Advertisements' },
  { number: 5, name: 'Create Contracts' },
];

export default function Setup() {
  const navigate = useNavigate();
  const [currentStep, setCurrentStep] = useState(1);
  const [isComplete, setIsComplete] = useState(false);

  const [orgAccountConfirmed, setOrgAccountConfirmed] = useState(false);
  const [envEnabledConfirmed, setEnvEnabledConfirmed] = useState(false);
  const [createdWorkgroups, setCreatedWorkgroups] = useState<Workgroup[]>([]);
  const [publishedAds, setPublishedAds] = useState<Advertisement[]>([]);
  const [createdContracts, setCreatedContracts] = useState<Contract[]>([]);
  const [skippedSteps, setSkippedSteps] = useState<ReadonlySet<number>>(new Set());

  function canContinue(): boolean {
    if (currentStep === 1) return orgAccountConfirmed;
    if (currentStep === 2) return envEnabledConfirmed;
    if (currentStep === 3) return createdWorkgroups.length > 0;
    return true;
  }

  function handleContinue() {
    if (currentStep < 5) {
      setCurrentStep((s) => s + 1);
    } else {
      setIsComplete(true);
    }
  }

  function handleSkip() {
    setSkippedSteps((prev) => new Set([...prev, currentStep]));
    if (currentStep < 5) {
      setCurrentStep((s) => s + 1);
    } else {
      setIsComplete(true);
    }
  }

  const currentStepName = isComplete
    ? 'Setup complete'
    : (STEPS.find((s) => s.number === currentStep)?.name ?? '');

  return (
    <div className="flex flex-col md:flex-row h-screen overflow-hidden bg-page text-text">
      {/* Left sidebar — desktop only */}
      <aside className="hidden md:flex w-[280px] shrink-0 flex-col border-r border-border bg-panel">
        <div className="border-b border-border p-6">
          <BrandMark product="agora" />
        </div>
        <nav className="flex flex-col gap-0.5 p-4" aria-label="Setup steps">
          {STEPS.map((step) => {
            const passed = isComplete || currentStep > step.number;
            const state: 'completed' | 'skipped' | 'active' | 'upcoming' = passed
              ? skippedSteps.has(step.number) ? 'skipped' : 'completed'
              : currentStep === step.number
                ? 'active'
                : 'upcoming';
            return (
              <StepSidebarItem
                key={step.number}
                step={step}
                state={state}
                onClick={passed ? () => { setCurrentStep(step.number); } : undefined}
              />
            );
          })}
        </nav>
      </aside>

      {/* Mobile step indicator — hidden on desktop */}
      <div className="md:hidden shrink-0 border-b border-border bg-panel px-4 py-3">
        <p className="text-[0.6875rem] font-medium uppercase tracking-[0.04em] text-text-mute">
          {isComplete ? 'Done' : `Step ${currentStep} of ${STEPS.length}`}
        </p>
        <p className="mt-0.5 text-[0.875rem] font-semibold text-text">{currentStepName}</p>
      </div>

      {/* Right column */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex-1 overflow-y-auto">
          <div className="mx-auto max-w-2xl px-4 py-4 md:px-8 md:py-8">
            {isComplete ? (
              <CompletionContent navigate={navigate} />
            ) : (
              <>
                {currentStep === 1 && (
                  <Step1Content
                    confirmed={orgAccountConfirmed}
                    onConfirmedChange={setOrgAccountConfirmed}
                  />
                )}
                {currentStep === 2 && (
                  <Step2Content
                    confirmed={envEnabledConfirmed}
                    onConfirmedChange={setEnvEnabledConfirmed}
                  />
                )}
                {currentStep === 3 && (
                  <Step3Content
                    createdWorkgroups={createdWorkgroups}
                    onWorkgroupCreated={(wg) => setCreatedWorkgroups((prev) => [...prev, wg])}
                  />
                )}
                {currentStep === 4 && (
                  <Step4Content
                    publishedAds={publishedAds}
                    workgroupsFromStep3={createdWorkgroups}
                    onAdPublished={(ad) => setPublishedAds((prev) => [...prev, ad])}
                  />
                )}
                {currentStep === 5 && (
                  <Step5Content
                    createdContracts={createdContracts}
                    onContractCreated={(c) => setCreatedContracts((prev) => [...prev, c])}
                  />
                )}
              </>
            )}
          </div>
        </div>

        {!isComplete && (
          <div className="shrink-0 border-t border-border bg-panel px-4 py-3 md:px-8 md:py-4 flex flex-wrap items-center gap-2 md:gap-3">
            {currentStep > 1 && (
              <Button
                type="button"
                variant="secondary"
                className="shrink-0"
                onClick={() => { setCurrentStep((s) => s - 1); }}
              >
                Back
              </Button>
            )}
            <div className="ml-auto flex shrink-0 items-center gap-2 md:gap-4">
              <button
                type="button"
                className="shrink-0 text-[0.8125rem] text-text-mute transition-colors hover:text-text"
                onClick={handleSkip}
              >
                Skip for now
              </button>
              <Button
                type="button"
                variant="primary"
                className="shrink-0"
                onClick={handleContinue}
                disabled={!canContinue()}
              >
                {currentStep === 5 ? 'Complete Setup' : 'Continue'}
              </Button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ── Sidebar step item ──────────────────────────────────────────────────────────

function StepSidebarItem({
  step,
  state,
  onClick,
}: {
  step: { number: number; name: string };
  state: 'completed' | 'skipped' | 'active' | 'upcoming';
  onClick?: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={!onClick}
      className={[
        'flex w-full items-center gap-3 rounded-[5px] px-3 py-2.5 text-left transition-colors',
        'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora',
        state === 'active' ? 'bg-brand-agora/10' : '',
        onClick ? 'cursor-pointer hover:bg-panel-subtle' : 'cursor-default',
      ]
        .filter(Boolean)
        .join(' ')}
    >
      <div
        className={[
          'flex size-7 shrink-0 items-center justify-center rounded-full text-[0.75rem] font-semibold',
          state === 'completed'
            ? 'bg-success/10 text-success'
            : state === 'skipped'
              ? 'bg-panel-subtle text-text-mute'
              : state === 'active'
                ? 'bg-brand-agora text-white'
                : 'bg-panel-subtle text-text-mute-2',
        ].join(' ')}
      >
        {state === 'completed' ? (
          <Check size={13} aria-hidden="true" />
        ) : state === 'skipped' ? (
          <Minus size={13} aria-hidden="true" />
        ) : (
          step.number
        )}
      </div>
      <span
        className={[
          'text-[0.8125rem]',
          state === 'active'
            ? 'font-bold text-brand-agora'
            : state === 'completed' || state === 'skipped'
              ? 'text-text-mute'
              : 'text-text-mute-2',
        ].join(' ')}
      >
        {step.name}
      </span>
    </button>
  );
}

// ── Completion state ───────────────────────────────────────────────────────────

function CompletionContent({ navigate }: { navigate: (path: string) => void }) {
  useEffect(() => {
    const timer = setTimeout(() => { navigate('/'); }, 3000);
    return () => clearTimeout(timer);
  }, [navigate]);

  return (
    <div className="flex flex-col items-center gap-6 py-16 text-center">
      <div className="flex size-16 items-center justify-center rounded-full bg-success/10 text-success">
        <CheckCircle2 size={36} aria-hidden="true" />
      </div>
      <div>
        <h1 className="text-[1.5rem] font-semibold text-text">You're all set</h1>
        <p className="mt-2 text-body text-text-mute">Your Agora network is ready.</p>
      </div>
      <Button type="button" variant="primary" onClick={() => { navigate('/'); }}>
        Go to Dashboard
      </Button>
    </div>
  );
}

// ── Shared helpers ─────────────────────────────────────────────────────────────

function StepHeader({
  title,
  subtitle,
  onInfoClick,
}: {
  title: string;
  subtitle: string;
  onInfoClick?: () => void;
}) {
  return (
    <div className="mb-6">
      <div className="flex flex-wrap items-start justify-between gap-2 md:gap-4">
        <h1 className="text-[1.25rem] font-semibold text-text">{title}</h1>
        {onInfoClick && (
          <button
            type="button"
            className="mt-0.5 flex shrink-0 items-center gap-1 text-[0.76rem] text-text-mute-2 transition-colors hover:text-brand-agora focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            onClick={onInfoClick}
            aria-label={`Learn more about ${title}`}
          >
            <Info size={13} aria-hidden="true" />
            <span>Learn more</span>
          </button>
        )}
      </div>
      <p className="mt-1 text-body text-text-mute">{subtitle}</p>
    </div>
  );
}

type CodeBlockProps = {
  label: string;
  lines: string[];
  copied: boolean;
  onCopy: () => void;
  reveal?: {
    revealed: boolean;
    onToggle: () => void;
    loading: boolean;
  };
};

function CodeBlock({ label, lines, copied, onCopy, reveal }: CodeBlockProps) {
  const displayLines = reveal && !reveal.revealed ? [TOKEN_MASK] : lines;
  return (
    <div className="overflow-hidden rounded-card border border-border bg-panel-subtle">
      <div className="flex items-center justify-between gap-3 border-b border-border px-3 py-2">
        <span className="text-label font-medium uppercase text-text-mute">{label}</span>
        <div className="flex items-center gap-0.5">
          {reveal && (
            <button
              type="button"
              onClick={reveal.onToggle}
              disabled={reveal.loading}
              aria-label={reveal.revealed ? 'Hide value' : 'Reveal value'}
              className="inline-flex size-7 items-center justify-center rounded-[5px] text-text-mute transition-colors hover:bg-panel hover:text-text disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            >
              {reveal.revealed ? (
                <EyeOff size={13} aria-hidden="true" />
              ) : (
                <Eye size={13} aria-hidden="true" />
              )}
            </button>
          )}
          <button
            type="button"
            onClick={onCopy}
            aria-label={copied ? 'Copied' : 'Copy to clipboard'}
            className="inline-flex size-7 items-center justify-center rounded-[5px] text-text-mute transition-colors hover:bg-panel hover:text-text focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          >
            {copied ? (
              <Check size={13} className="text-success" aria-hidden="true" />
            ) : (
              <Copy size={13} aria-hidden="true" />
            )}
          </button>
        </div>
      </div>
      <pre className="overflow-x-auto px-3 py-2.5 font-mono text-[0.76rem] leading-relaxed text-text whitespace-pre-wrap break-all">
        {displayLines.join('\n')}
      </pre>
    </div>
  );
}

function InlineFormContainer({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-card border border-border bg-panel p-4">
      {children}
    </div>
  );
}

function FormField({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={htmlFor} className="text-label font-medium uppercase text-text-mute">
        {label}
      </label>
      {children}
    </div>
  );
}

function InlineError({ message }: { message: string }) {
  return <p className="text-[0.76rem] text-danger">{message}</p>;
}

function AddAnotherLink({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-fit items-center gap-1 text-[0.8125rem] text-brand-agora transition-colors hover:text-brand-agora-end focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
    >
      <Plus size={14} aria-hidden="true" />
      {label}
    </button>
  );
}

function errorDetail(error: unknown): string {
  if (error instanceof ApiError) {
    const status = error.status ? `${error.status} ` : '';
    const code = error.code ? `${error.code}: ` : '';
    return `${status}${code}${error.message}`;
  }
  if (error instanceof Error) return error.message;
  return 'request failed';
}

function copyToClipboard(text: string): Promise<void> {
  return navigator.clipboard.writeText(text);
}

// ── Step 1: Organization and Account ──────────────────────────────────────────

const ORG_BLOCK_LINES = [
  'export AGORA_ADMIN_TOKEN=your-admin-token',
  'agora admin create organization <your-org-name>',
];
const USER_BLOCK_LINES = ['agora admin create user <organizationId> <email> <password>'];

function Step1Content({
  confirmed,
  onConfirmedChange,
}: {
  confirmed: boolean;
  onConfirmedChange: (v: boolean) => void;
}) {
  const [copied1, setCopied1] = useState(false);
  const [copied2, setCopied2] = useState(false);

  function flashCopy(text: string, setFlag: (v: boolean) => void) {
    void copyToClipboard(text).then(() => {
      setFlag(true);
      setTimeout(() => { setFlag(false); }, 1500);
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <StepHeader
        title="Create your organization and account"
        subtitle="These are admin operations performed via the CLI."
      />
      <p className="text-body text-text-mute">
        Before using Agora, an admin needs to create your organization and at least one account via the CLI.
      </p>
      <div className="flex flex-col gap-3">
        <CodeBlock
          label="Create organization"
          lines={ORG_BLOCK_LINES}
          copied={copied1}
          onCopy={() => { flashCopy(ORG_BLOCK_LINES.join('\n'), setCopied1); }}
        />
        <CodeBlock
          label="Create account"
          lines={USER_BLOCK_LINES}
          copied={copied2}
          onCopy={() => { flashCopy(USER_BLOCK_LINES.join('\n'), setCopied2); }}
        />
      </div>
      <label className="flex cursor-pointer items-center gap-3">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={(e) => { onConfirmedChange(e.target.checked); }}
          className="h-4 w-4 rounded border-border accent-brand-agora"
        />
        <span className="text-body text-text-mute-strong">
          I've created my organization and account
        </span>
      </label>
    </div>
  );
}

// ── Step 2: Enroll Environment ─────────────────────────────────────────────────

const CONFIG_BLOCK_LINES = ['agora config set api_endpoint <controller-endpoint>'];

function Step2Content({
  confirmed,
  onConfirmedChange,
}: {
  confirmed: boolean;
  onConfirmedChange: (v: boolean) => void;
}) {
  const [accountToken, setAccountToken] = useState<string | null>(null);
  const [tokenRevealed, setTokenRevealed] = useState(false);
  const [tokenLoading, setTokenLoading] = useState(false);
  const [tokenError, setTokenError] = useState<string | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const ensureToken = useCallback(async (): Promise<string | null> => {
    if (accountToken) return accountToken;
    setTokenLoading(true);
    setTokenError(null);
    try {
      const res = await getAccountToken();
      setAccountToken(res.accountToken);
      return res.accountToken;
    } catch (err) {
      setTokenError(errorDetail(err));
      return null;
    } finally {
      setTokenLoading(false);
    }
  }, [accountToken]);

  const flashCopied = useCallback((field: string) => {
    setCopiedField(field);
    setTimeout(() => { setCopiedField((c) => (c === field ? null : c)); }, 1500);
  }, []);

  const handleCopyConfig = useCallback(() => {
    void copyToClipboard(CONFIG_BLOCK_LINES.join('\n')).then(() => { flashCopied('config'); });
  }, [flashCopied]);

  const handleToggleReveal = useCallback(async () => {
    if (tokenRevealed) { setTokenRevealed(false); return; }
    const value = await ensureToken();
    if (value) setTokenRevealed(true);
  }, [tokenRevealed, ensureToken]);

  const handleCopyToken = useCallback(async () => {
    const value = await ensureToken();
    if (!value) return;
    await copyToClipboard(value);
    flashCopied('token');
  }, [ensureToken, flashCopied]);

  const handleCopyCommand = useCallback(async () => {
    const value = await ensureToken();
    if (!value) return;
    await copyToClipboard(`agora enable ${value}`);
    flashCopied('command');
  }, [ensureToken, flashCopied]);

  const tokenDisplay = tokenRevealed && accountToken ? accountToken : TOKEN_MASK;
  const commandDisplay =
    tokenRevealed && accountToken ? `agora enable ${accountToken}` : `agora enable ${TOKEN_MASK}`;

  return (
    <div className="flex flex-col gap-5">
      <StepHeader
        title="Enroll your first environment"
        subtitle="An environment is a provisioned network presence for your agents."
      />
      <p className="text-body text-text-mute">
        Environments are enrolled locally with the Agora CLI on the machine where your agent runs —
        this cannot be done from the dashboard. Run the commands below on that machine.
      </p>

      <div className="flex flex-col gap-2">
        <CodeBlock
          label="Point the CLI at the controller"
          lines={CONFIG_BLOCK_LINES}
          copied={copiedField === 'config'}
          onCopy={handleCopyConfig}
        />
        <p className="text-[0.76rem] text-text-mute">
          Run once to tell the CLI which controller to use. Agora stores local settings under{' '}
          <code className="font-mono">~/.agora</code>.
        </p>
      </div>

      <div className="flex flex-col gap-3">
        <CodeBlock
          label="Account token"
          lines={[tokenDisplay]}
          copied={copiedField === 'token'}
          onCopy={() => { void handleCopyToken(); }}
          reveal={{
            revealed: tokenRevealed,
            onToggle: () => { void handleToggleReveal(); },
            loading: tokenLoading,
          }}
        />
        <div className="flex flex-col gap-2">
          <CodeBlock
            label="Enable the environment"
            lines={[commandDisplay]}
            copied={copiedField === 'command'}
            onCopy={() => { void handleCopyCommand(); }}
          />
          <p className="text-[0.76rem] text-text-mute">
            Provide the account token above, or omit the token to log in interactively with your
            account email and password. Optional flags: <code className="font-mono">--description</code>,{' '}
            <code className="font-mono">--host</code>. This enrolls the environment and writes its
            identity material under <code className="font-mono">~/.agora</code>.
          </p>
        </div>
        {tokenError && <InlineError message={tokenError} />}
      </div>

      <label className="flex cursor-pointer items-center gap-3">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={(e) => { onConfirmedChange(e.target.checked); }}
          className="h-4 w-4 rounded border-border accent-brand-agora"
        />
        <span className="text-body text-text-mute-strong">
          I've enabled my environment
        </span>
      </label>
    </div>
  );
}

// ── Step 3: Create Workgroups ──────────────────────────────────────────────────

function Step3Content({
  createdWorkgroups,
  onWorkgroupCreated,
}: {
  createdWorkgroups: Workgroup[];
  onWorkgroupCreated: (wg: Workgroup) => void;
}) {
  const [showForm, setShowForm] = useState(true);
  const [wgName, setWgName] = useState('');
  const [wgScope, setWgScope] = useState<WorkgroupScope>('intra-org');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [infoOpen, setInfoOpen] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const name = wgName.trim();
    if (!name) return;
    setSubmitting(true);
    setFormError(null);
    try {
      const wg = await createWorkgroup({ name, scope: wgScope });
      onWorkgroupCreated(wg);
      setWgName('');
      setWgScope('intra-org');
      setShowForm(false);
    } catch (err) {
      setFormError(errorDetail(err));
    } finally {
      setSubmitting(false);
    }
  }

  const hasCreated = createdWorkgroups.length > 0;

  return (
    <div className="flex flex-col gap-5">
      <StepHeader
        title="Create your workgroups"
        subtitle="Workgroups are policy boundaries that control which agents can discover and interact with each other."
        onInfoClick={() => { setInfoOpen(true); }}
      />

      {/* Created workgroup cards */}
      {createdWorkgroups.map((wg) => (
        <div key={wg.id} className="flex items-center gap-3 rounded-card border border-border bg-panel p-3">
          <div className="min-w-0 flex-1">
            <p className="font-semibold text-text">{wg.name}</p>
            <p className="mt-0.5 font-mono text-[0.76rem] text-text-mute">{wg.id}</p>
          </div>
          <StatusPill
            status={wg.scope === 'inter-org' ? 'info' : 'success'}
            label={wg.scope}
          />
        </div>
      ))}

      {/* "Add another workgroup" link */}
      {hasCreated && !showForm && (
        <AddAnotherLink label="Add another workgroup" onClick={() => { setShowForm(true); }} />
      )}

      {infoOpen && <WorkgroupsInfoDrawer onClose={() => { setInfoOpen(false); }} />}

      {/* Inline form */}
      {showForm && (
        <InlineFormContainer>
          <form onSubmit={(e) => { void handleSubmit(e); }} className="flex flex-col gap-4">
            <FormField label="Name" htmlFor="wg-name">
              <Input
                id="wg-name"
                type="text"
                value={wgName}
                onChange={(e) => { setWgName(e.target.value); }}
                placeholder="my-workgroup"
                required
                disabled={submitting}
                autoFocus
              />
            </FormField>
            <div className="flex flex-col gap-1.5">
              <span className="text-label font-medium uppercase text-text-mute">Scope</span>
              <div className="flex rounded-[5px] border border-border overflow-hidden w-fit">
                {(['intra-org', 'inter-org'] as WorkgroupScope[]).map((scope, i) => (
                  <button
                    key={scope}
                    type="button"
                    onClick={() => { setWgScope(scope); }}
                    disabled={submitting}
                    className={[
                      'px-4 py-1.5 text-[0.76rem] font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-brand-agora disabled:opacity-50',
                      i > 0 ? 'border-l border-border' : '',
                      wgScope === scope
                        ? 'bg-brand-agora text-white'
                        : 'bg-panel-subtle text-text-mute hover:bg-border-light',
                    ]
                      .filter(Boolean)
                      .join(' ')}
                  >
                    {scope === 'intra-org' ? 'Intra-org' : 'Inter-org'}
                  </button>
                ))}
              </div>
            </div>
            {formError && <InlineError message={formError} />}
            <div className="flex gap-2">
              <Button type="submit" disabled={submitting || !wgName.trim()}>
                {submitting ? 'Creating…' : 'Add Workgroup'}
              </Button>
              {hasCreated && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => { setShowForm(false); setFormError(null); }}
                >
                  Cancel
                </Button>
              )}
            </div>
          </form>
        </InlineFormContainer>
      )}
    </div>
  );
}

// ── Step 4: Publish Advertisements ────────────────────────────────────────────

function Step4Content({
  publishedAds,
  workgroupsFromStep3,
  onAdPublished,
}: {
  publishedAds: Advertisement[];
  workgroupsFromStep3: Workgroup[];
  onAdPublished: (ad: Advertisement) => void;
}) {
  const [showForm, setShowForm] = useState(true);
  const [adName, setAdName] = useState('');
  const [adDescription, setAdDescription] = useState('');
  const [adCapability, setAdCapability] = useState('');
  const [adInteractionPattern, setAdInteractionPattern] =
    useState<AdvertisementInteractionPatternKind>('request-response');
  const [adTunnelMode, setAdTunnelMode] = useState<AdvertisementTunnelMode>('tcp');
  const [adWorkgroupId, setAdWorkgroupId] = useState('');
  const [adContractId, setAdContractId] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const [workgroups, setWorkgroups] = useState<Workgroup[]>([]);
  const [contracts, setContracts] = useState<Contract[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [infoOpen, setInfoOpen] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([listWorkgroups(controller.signal), listContracts(controller.signal)])
      .then(([wgs, cts]) => {
        if (controller.signal.aborted) return;
        const merged = mergeWorkgroups(workgroupsFromStep3, wgs);
        setWorkgroups(merged);
        setContracts(cts);
        if (!adWorkgroupId && merged.length > 0) {
          setAdWorkgroupId(merged[0]!.id);
        }
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) setLoadError(errorDetail(err));
      });
    return () => { controller.abort(); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const name = adName.trim();
    const capability = adCapability.trim();
    if (!name || !capability || !adWorkgroupId) return;
    setSubmitting(true);
    setFormError(null);
    try {
      const ad = await createAdvertisement({
        name,
        description: adDescription.trim() || undefined,
        capabilities: [{ name: capability }],
        interactionPatterns: [{ kind: adInteractionPattern }],
        tunnelMode: adTunnelMode,
        workgroupScopes: [adWorkgroupId],
        contractId: adContractId || undefined,
      });
      onAdPublished(ad);
      setAdName('');
      setAdDescription('');
      setAdCapability('');
      setAdInteractionPattern('request-response');
      setAdTunnelMode('tcp');
      setAdWorkgroupId(workgroups[0]?.id ?? '');
      setAdContractId('');
      setShowForm(false);
    } catch (err) {
      setFormError(errorDetail(err));
    } finally {
      setSubmitting(false);
    }
  }

  const hasPublished = publishedAds.length > 0;

  return (
    <div className="flex flex-col gap-5">
      <StepHeader
        title="Publish your advertisements"
        subtitle="Advertisements declare what your agents offer to the network."
        onInfoClick={() => { setInfoOpen(true); }}
      />

      {/* Published advertisement cards */}
      {publishedAds.map((ad) => (
        <div
          key={ad.id}
          className="relative flex overflow-hidden rounded-card border border-border bg-panel p-3"
        >
          <div className="min-w-0 flex-1 pr-4">
            <p className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] text-text-mute-strong">
              {ad.organizationName}
            </p>
            <p className="truncate font-semibold text-text">{ad.name}</p>
            <p className="font-mono text-[0.6875rem] text-text-mute-2">{ad.id}</p>
            {ad.description && (
              <p className="mt-1 text-[0.76rem] text-text-mute">{ad.description}</p>
            )}
            {ad.capabilities.length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {ad.capabilities.map((cap) => (
                  <span
                    key={cap.name}
                    className="inline-flex items-center rounded-status border border-brand-agora/30 bg-brand-agora/10 px-2 py-0.5 text-[0.6875rem] font-medium text-brand-agora"
                  >
                    {cap.name}
                  </span>
                ))}
              </div>
            )}
          </div>
          <StatusPill status="info" label={ad.tunnelMode ?? 'tcp'} className="shrink-0" />
        </div>
      ))}

      {/* "Add another advertisement" link */}
      {hasPublished && !showForm && (
        <AddAnotherLink label="Add another advertisement" onClick={() => { setShowForm(true); }} />
      )}

      {loadError && <InlineError message={`Failed to load options: ${loadError}`} />}

      {infoOpen && <AdvertisementsInfoDrawer onClose={() => { setInfoOpen(false); }} />}

      {/* Inline form */}
      {showForm && (
        <InlineFormContainer>
          <form onSubmit={(e) => { void handleSubmit(e); }} className="flex flex-col gap-4">
            <FormField label="Name" htmlFor="ad-name">
              <Input
                id="ad-name"
                type="text"
                value={adName}
                onChange={(e) => { setAdName(e.target.value); }}
                placeholder="my-agent"
                required
                disabled={submitting}
                autoFocus
              />
            </FormField>
            <FormField label="Description (optional)" htmlFor="ad-description">
              <Input
                id="ad-description"
                type="text"
                value={adDescription}
                onChange={(e) => { setAdDescription(e.target.value); }}
                placeholder="What this agent does"
                disabled={submitting}
              />
            </FormField>
            <FormField label="Capability" htmlFor="ad-capability">
              <Input
                id="ad-capability"
                type="text"
                value={adCapability}
                onChange={(e) => { setAdCapability(e.target.value); }}
                placeholder="domain.capability e.g. markets.equity"
                required
                disabled={submitting}
              />
            </FormField>
            <FormField label="Interaction pattern" htmlFor="ad-interaction">
              <div className="relative">
                <Select
                  id="ad-interaction"
                  value={adInteractionPattern}
                  onChange={(e) => { setAdInteractionPattern(e.target.value as AdvertisementInteractionPatternKind); }}
                  disabled={submitting}
                  className="w-full"
                >
                  <option value="request-response">request-response</option>
                  <option value="stream">stream</option>
                  <option value="broadcast">broadcast</option>
                  <option value="custom">custom</option>
                </Select>
              </div>
            </FormField>
            <FormField label="Protocol" htmlFor="ad-tunnel">
              <div className="relative">
                <Select
                  id="ad-tunnel"
                  value={adTunnelMode}
                  onChange={(e) => { setAdTunnelMode(e.target.value as AdvertisementTunnelMode); }}
                  disabled={submitting}
                  className="w-full"
                >
                  <option value="tcp">tcp</option>
                  <option value="udp">udp</option>
                  <option value="http">http</option>
                </Select>
              </div>
            </FormField>
            <FormField label="Workgroup" htmlFor="ad-workgroup">
              <div className="relative">
                <Select
                  id="ad-workgroup"
                  value={adWorkgroupId}
                  onChange={(e) => { setAdWorkgroupId(e.target.value); }}
                  disabled={submitting || workgroups.length === 0}
                  className="w-full"
                  required
                >
                  {workgroups.length === 0 && (
                    <option value="">No workgroups available</option>
                  )}
                  {workgroups.map((wg) => (
                    <option key={wg.id} value={wg.id}>
                      {wg.name}
                    </option>
                  ))}
                </Select>
              </div>
            </FormField>
            <FormField label="Contract (optional)" htmlFor="ad-contract">
              <div className="relative">
                <Select
                  id="ad-contract"
                  value={adContractId}
                  onChange={(e) => { setAdContractId(e.target.value); }}
                  disabled={submitting}
                  className="w-full"
                >
                  <option value="">None</option>
                  {contracts.map((ct) => (
                    <option key={ct.id} value={ct.id}>
                      {ct.name}
                    </option>
                  ))}
                </Select>
              </div>
            </FormField>
            {formError && <InlineError message={formError} />}
            <div className="flex gap-2">
              <Button
                type="submit"
                disabled={submitting || !adName.trim() || !adCapability.trim() || !adWorkgroupId}
              >
                {submitting ? 'Publishing…' : 'Add Advertisement'}
              </Button>
              {hasPublished && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => { setShowForm(false); setFormError(null); }}
                >
                  Cancel
                </Button>
              )}
            </div>
          </form>
        </InlineFormContainer>
      )}
    </div>
  );
}

// ── Step 5: Create Contracts ───────────────────────────────────────────────────

function Step5Content({
  createdContracts,
  onContractCreated,
}: {
  createdContracts: Contract[];
  onContractCreated: (c: Contract) => void;
}) {
  const [showForm, setShowForm] = useState(true);
  const [ctName, setCtName] = useState('');
  const [ctDescription, setCtDescription] = useState('');
  const [ctAccessMode, setCtAccessMode] = useState<ContractAccessMode>('open');
  const [ctMaxDuration, setCtMaxDuration] = useState('');
  const [ctMaxEnvelopes, setCtMaxEnvelopes] = useState('');
  const [ctMaxBytes, setCtMaxBytes] = useState('');
  const [ctMessageTypes, setCtMessageTypes] = useState<string[]>([]);
  const [ctMessageTypeInput, setCtMessageTypeInput] = useState('');
  const [ctMinAccountAgeDays, setCtMinAccountAgeDays] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [infoOpen, setInfoOpen] = useState(false);

  function addMessageType() {
    const value = ctMessageTypeInput.trim();
    if (value && !ctMessageTypes.includes(value)) {
      setCtMessageTypes((prev) => [...prev, value]);
    }
    setCtMessageTypeInput('');
  }

  function removeMessageType(type: string) {
    setCtMessageTypes((prev) => prev.filter((t) => t !== type));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const name = ctName.trim();
    if (!name) return;
    setSubmitting(true);
    setFormError(null);
    try {
      const contract = await createContract({
        name,
        description: ctDescription.trim() || undefined,
        accessMode: ctAccessMode,
        maxDurationSeconds: ctMaxDuration ? parseInt(ctMaxDuration, 10) : undefined,
        maxEnvelopeCount: ctMaxEnvelopes ? parseInt(ctMaxEnvelopes, 10) : undefined,
        maxEnvelopeBytes: ctMaxBytes ? parseInt(ctMaxBytes, 10) : undefined,
        allowedMessageTypes: ctMessageTypes.length > 0 ? ctMessageTypes : undefined,
        maturityRequirements: ctMinAccountAgeDays
          ? { minAccountAgeDays: parseInt(ctMinAccountAgeDays, 10) }
          : undefined,
      });
      onContractCreated(contract);
      setCtName('');
      setCtDescription('');
      setCtAccessMode('open');
      setCtMaxDuration('');
      setCtMaxEnvelopes('');
      setCtMaxBytes('');
      setCtMessageTypes([]);
      setCtMessageTypeInput('');
      setCtMinAccountAgeDays('');
      setShowForm(false);
    } catch (err) {
      setFormError(errorDetail(err));
    } finally {
      setSubmitting(false);
    }
  }

  const hasCreated = createdContracts.length > 0;

  return (
    <div className="flex flex-col gap-5">
      <StepHeader
        title="Create your contracts"
        subtitle="Contracts define the engagement terms that govern every session on your network."
        onInfoClick={() => { setInfoOpen(true); }}
      />

      {/* Created contract cards */}
      {createdContracts.map((ct) => (
        <div key={ct.id} className="rounded-card border border-border bg-panel p-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 flex-1">
              <p className="font-semibold text-text">{ct.name}</p>
              <p className="font-mono text-[0.6875rem] text-text-mute-2">{ct.id}</p>
              {ct.description && (
                <p className="mt-1 text-[0.76rem] text-text-mute">{ct.description}</p>
              )}
            </div>
            <StatusPill
              status={ct.accessMode === 'open' ? 'success' : 'warning'}
              label={ct.accessMode.replace(/_/g, ' ')}
            />
          </div>
        </div>
      ))}

      {/* "Add another contract" link */}
      {hasCreated && !showForm && (
        <AddAnotherLink label="Add another contract" onClick={() => { setShowForm(true); }} />
      )}

      {infoOpen && <ContractsInfoDrawer onClose={() => { setInfoOpen(false); }} />}

      {/* Inline form */}
      {showForm && (
        <InlineFormContainer>
          <form onSubmit={(e) => { void handleSubmit(e); }} className="flex flex-col gap-4">
            <FormField label="Name" htmlFor="ct-name">
              <Input
                id="ct-name"
                type="text"
                value={ctName}
                onChange={(e) => { setCtName(e.target.value); }}
                placeholder="my-contract"
                required
                disabled={submitting}
                autoFocus
              />
            </FormField>
            <FormField label="Description (optional)" htmlFor="ct-description">
              <Input
                id="ct-description"
                type="text"
                value={ctDescription}
                onChange={(e) => { setCtDescription(e.target.value); }}
                placeholder="Contract purpose"
                disabled={submitting}
              />
            </FormField>
            <FormField label="Access mode" htmlFor="ct-access">
              <div className="relative">
                <Select
                  id="ct-access"
                  value={ctAccessMode}
                  onChange={(e) => { setCtAccessMode(e.target.value as ContractAccessMode); }}
                  disabled={submitting}
                  className="w-full"
                >
                  <option value="open">open</option>
                  <option value="approval_required">approval_required</option>
                </Select>
              </div>
            </FormField>
            <FormField label="Max duration (seconds, optional)" htmlFor="ct-duration">
              <Input
                id="ct-duration"
                type="number"
                min={0}
                value={ctMaxDuration}
                onChange={(e) => { setCtMaxDuration(e.target.value); }}
                placeholder="No limit"
                disabled={submitting}
              />
            </FormField>
            <FormField label="Max envelopes (optional)" htmlFor="ct-envelopes">
              <Input
                id="ct-envelopes"
                type="number"
                min={0}
                value={ctMaxEnvelopes}
                onChange={(e) => { setCtMaxEnvelopes(e.target.value); }}
                placeholder="No limit"
                disabled={submitting}
              />
            </FormField>
            <FormField label="Max bytes (optional)" htmlFor="ct-bytes">
              <Input
                id="ct-bytes"
                type="number"
                min={0}
                value={ctMaxBytes}
                onChange={(e) => { setCtMaxBytes(e.target.value); }}
                placeholder="No limit"
                disabled={submitting}
              />
            </FormField>
            <div className="flex flex-col gap-1.5">
              <label className="text-label font-medium uppercase text-text-mute">
                Allowed message types (optional)
              </label>
              {ctMessageTypes.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {ctMessageTypes.map((type) => (
                    <span
                      key={type}
                      className="inline-flex items-center gap-1 rounded-status border border-border bg-panel-subtle px-2 py-0.5 text-[0.76rem] text-text-mute-strong"
                    >
                      {type}
                      <button
                        type="button"
                        onClick={() => { removeMessageType(type); }}
                        aria-label={`Remove ${type}`}
                        className="text-text-mute hover:text-danger focus-visible:outline-none"
                      >
                        <X size={11} aria-hidden="true" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
              <div className="flex gap-2">
                <Input
                  type="text"
                  value={ctMessageTypeInput}
                  onChange={(e) => { setCtMessageTypeInput(e.target.value); }}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') { e.preventDefault(); addMessageType(); }
                  }}
                  placeholder="markets.equity.request — press Enter to add"
                  disabled={submitting}
                  className="flex-1"
                />
                <Button
                  type="button"
                  variant="secondary"
                  onClick={addMessageType}
                  disabled={!ctMessageTypeInput.trim()}
                >
                  Add
                </Button>
              </div>
            </div>
            <FormField label="Minimum account age (days), optional" htmlFor="ct-min-account-age">
              <Input
                id="ct-min-account-age"
                type="number"
                min={0}
                value={ctMinAccountAgeDays}
                onChange={(e) => { setCtMinAccountAgeDays(e.target.value); }}
                placeholder="No requirement"
                disabled={submitting}
              />
            </FormField>
            {formError && <InlineError message={formError} />}
            <div className="flex gap-2">
              <Button type="submit" disabled={submitting || !ctName.trim()}>
                {submitting ? 'Creating…' : 'Add Contract'}
              </Button>
              {hasCreated && (
                <Button
                  type="button"
                  variant="secondary"
                  onClick={() => { setShowForm(false); setFormError(null); }}
                >
                  Cancel
                </Button>
              )}
            </div>
          </form>
        </InlineFormContainer>
      )}
    </div>
  );
}

// ── Info drawers ───────────────────────────────────────────────────────────────

function WorkgroupsInfoDrawer({ onClose }: { onClose: () => void }) {
  return (
    <InfoDrawer title="About Workgroups" onClose={onClose}>
      <div className="flex flex-col gap-5">
        <section>
          <h3 className="mb-2 font-semibold text-text">What is a Workgroup?</h3>
          <p className="leading-relaxed text-text-mute">
            Workgroups are the primary policy boundary in Agora. They control visibility and
            interaction scope at the discovery layer. An agent outside a workgroup cannot discover
            agents inside it, cannot query their capabilities, and has zero knowledge of their
            existence — this is not a filtered view, it is structural invisibility enforced by the controller.
          </p>
        </section>

        <DrawerDivider />

        <section>
          <h3 className="mb-3 font-semibold text-text">Intra-org vs. Inter-org</h3>
          <div className="flex flex-col gap-2">
            <DrawerCard
              icon={Building2}
              title="Intra-Organization"
              description="Define internal team boundaries within a single organization. Members share resources and visibility within their org."
            />
            <DrawerCard
              icon={ArrowLeftRight}
              title="Inter-Organization"
              description="Enable cross-company collaboration channels, established via explicit invitation handshakes. Each organization independently manages its own members. No cross-org admin authority."
            />
          </div>
        </section>

        <DrawerDivider />

        <section>
          <h3 className="mb-3 font-semibold text-text">What workgroup membership controls</h3>
          <div className="flex flex-col gap-2">
            <DrawerCard
              icon={BookOpen}
              title="Catalog Visibility"
              description="Controls which advertisements appear in discovery queries. Agents outside the workgroup cannot see advertisements scoped to it."
            />
            <DrawerCard
              icon={ArrowLeftRight}
              title="Session Proposals"
              description="Controls which agents can propose sessions to which other agents. Only members with shared workgroup access can initiate a session."
            />
            <DrawerCard
              icon={Zap}
              title="Active Session Continuity"
              description="If an agent loses workgroup membership, active sessions with members of that workgroup are closed immediately."
            />
          </div>
        </section>

        <DrawerDivider />

        <DrawerTip>
          When an agent loses workgroup access, advertisements vanish from its catalog view instantly.
          Active sessions with members of that workgroup are closed immediately with a recorded close reason.
        </DrawerTip>
      </div>
    </InfoDrawer>
  );
}

function AdvertisementsInfoDrawer({ onClose }: { onClose: () => void }) {
  return (
    <InfoDrawer title="About Advertisements" onClose={onClose}>
      <div className="flex flex-col gap-5">
        <section>
          <h3 className="mb-2 font-semibold text-text">How Advertisements work in the Catalog</h3>
          <p className="leading-relaxed text-text-mute">
            The Catalog is Agora's built-in discovery surface. Agents query it to find other agents
            and their capabilities. Every catalog query is filtered by the caller's workgroup
            memberships — entries you are not entitled to see do not appear in results at all.
            The Catalog is an intrinsic controller service, not a separate registry to configure or maintain.
          </p>
        </section>

        <DrawerDivider />

        <section>
          <h3 className="mb-3 font-semibold text-text">What an Advertisement contains</h3>
          <div className="flex flex-col gap-2">
            <DrawerCard
              icon={Tag}
              title="Name and capabilities"
              description={
                <>
                  Declared service name and capability identifiers, e.g.{' '}
                  <DrawerCodeChip>domain.capability</DrawerCodeChip>,{' '}
                  <DrawerCodeChip>provider.service</DrawerCodeChip>.
                </>
              }
            />
            <DrawerCard
              icon={Globe}
              title="Visibility scope"
              description="Which workgroups can discover this advertisement. Agents in other workgroups see nothing."
            />
            <DrawerCard
              icon={Network}
              title="Tunnel protocol"
              description={
                <>
                  Supported transport: <DrawerCodeChip>http</DrawerCodeChip>,{' '}
                  <DrawerCodeChip>tcp</DrawerCodeChip>, or <DrawerCodeChip>udp</DrawerCodeChip>.
                </>
              }
            />
            <DrawerCard
              icon={FileCheck2}
              title="Contract terms"
              description="The required contract any consumer must accept to open a session with this agent."
            />
          </div>
        </section>

        <DrawerDivider />

        <section>
          <h3 className="mb-2 font-semibold text-text">How discovery works</h3>
          <p className="leading-relaxed text-text-mute">
            An agent calls the catalog search. Results are limited to advertisements published by
            agents in workgroups the caller shares. Agents in workgroups the caller does not share
            are fully absent — not hidden, not redacted, not visible at all.
          </p>
        </section>

        <DrawerDivider />

        <DrawerTip>
          When an agent loses workgroup access, its advertisements vanish from the catalog immediately — no cache invalidation delay.
        </DrawerTip>
      </div>
    </InfoDrawer>
  );
}

function ContractsInfoDrawer({ onClose }: { onClose: () => void }) {
  return (
    <InfoDrawer title="About Contracts" onClose={onClose}>
      <div className="flex flex-col gap-5">
        <section>
          <h3 className="mb-2 font-semibold text-text">What is a Contract?</h3>
          <p className="leading-relaxed text-text-mute">
            A Contract defines the engagement terms for a session before it begins. Contracts are
            evaluated by the controller at session establishment time and enforced throughout the
            session. The agents themselves do not need to implement any enforcement logic.
          </p>
        </section>

        <DrawerDivider />

        <section>
          <h3 className="mb-3 font-semibold text-text">Contract Terms</h3>
          <div className="flex flex-col gap-2">
            <DrawerCard
              icon={Clock}
              title="max_duration_seconds"
              description={
                <>
                  Maximum session lifetime. Exceeded sessions are closed by the controller with a
                  recorded close reason. Field: <DrawerCodeChip>max_duration_seconds</DrawerCodeChip>
                </>
              }
            />
            <DrawerCard
              icon={Hash}
              title="max_envelope_count"
              description={
                <>
                  Maximum number of envelopes that can be exchanged. Attempts beyond this limit are
                  rejected. Field: <DrawerCodeChip>max_envelope_count</DrawerCodeChip>
                </>
              }
            />
            <DrawerCard
              icon={MessageSquare}
              title="allowed_message_types"
              description={
                <>
                  The set of message types permitted in this session. Any envelope with a type
                  outside this set is rejected at the controller. Field:{' '}
                  <DrawerCodeChip>allowed_message_types</DrawerCodeChip>
                </>
              }
            />
            <DrawerCard
              icon={ShieldCheck}
              title="Required workgroup memberships"
              description="Preconditions that must be satisfied for the session to be established."
            />
            <DrawerCard
              icon={Lock}
              title="access_mode"
              description={
                <>
                  Controls how sessions are established.{' '}
                  <DrawerCodeChip>approval_required</DrawerCodeChip> means the provider must
                  explicitly accept before the session goes active.
                </>
              }
            />
          </div>
        </section>

        <DrawerDivider />

        <DrawerTip>
          Providers do not need to be online to decide whether to accept an engagement — the contract
          speaks for them. Governance is structural, not dependent on agent-side logic.
        </DrawerTip>
      </div>
    </InfoDrawer>
  );
}

// ── Utilities ──────────────────────────────────────────────────────────────────

function mergeWorkgroups(fromStep: Workgroup[], fromApi: Workgroup[]): Workgroup[] {
  const seen = new Set(fromStep.map((wg) => wg.id));
  const merged = [...fromStep];
  for (const wg of fromApi) {
    if (!seen.has(wg.id)) {
      merged.push(wg);
      seen.add(wg.id);
    }
  }
  return merged;
}

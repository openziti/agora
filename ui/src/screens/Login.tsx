import { type FormEvent, useState } from 'react';
import { useNavigate } from 'react-router';
import { AlertCircle, LoaderCircle, LogIn } from 'lucide-react';

import { BrandMark } from '../components';
import { ApiError, getWhoami, listEnvironments, login } from '../lib/api';
import { setAuthenticatedAccount } from '../lib/auth-state';
import { SETUP_ENABLED } from '../lib/config';

export default function Login() {
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [errorMessage, setErrorMessage] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    setErrorMessage(undefined);
    setSubmitting(true);

    try {
      const loginResponse = await login({ email: email.trim(), password });
      void loginResponse.accountToken;

      const account = await getWhoami();
      setAuthenticatedAccount(account);
      const environments = await listEnvironments();
      navigate(SETUP_ENABLED && environments.length === 0 ? '/setup' : '/', { replace: true });
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setErrorMessage('Email or password is incorrect.');
      } else {
        setErrorMessage('Unable to sign in. Please try again.');
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-page px-6 py-10 text-text">
      <section className="w-full max-w-[26rem] rounded-card border border-border bg-panel p-6 shadow-sm">
        <BrandMark product="agora" className="justify-center" />

        <form className="mt-8 flex flex-col gap-5" onSubmit={handleSubmit}>
          <div className="flex flex-col gap-2">
            <label htmlFor="email" className="text-label font-medium uppercase text-text-mute">
              Email
            </label>
            <input
              id="email"
              name="email"
              type="email"
              autoComplete="username"
              value={email}
              disabled={submitting}
              required
              onChange={(event) => setEmail(event.target.value)}
              className="h-10 w-full rounded-pill border border-border bg-panel-subtle px-3 text-body text-text outline-none placeholder:text-text-mute disabled:cursor-not-allowed disabled:text-text-mute focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            />
          </div>

          <div className="flex flex-col gap-2">
            <label htmlFor="password" className="text-label font-medium uppercase text-text-mute">
              Password
            </label>
            <input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              value={password}
              disabled={submitting}
              required
              onChange={(event) => setPassword(event.target.value)}
              className="h-10 w-full rounded-pill border border-border bg-panel-subtle px-3 text-body text-text outline-none placeholder:text-text-mute disabled:cursor-not-allowed disabled:text-text-mute focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-pill bg-brand-agora px-4 text-body font-semibold text-white hover:bg-brand-agora-end disabled:cursor-not-allowed disabled:opacity-70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-agora"
          >
            {submitting ? (
              <LoaderCircle size={17} aria-hidden="true" className="animate-spin" />
            ) : (
              <LogIn size={17} aria-hidden="true" />
            )}
            <span>{submitting ? 'Signing in' : 'Sign in'}</span>
          </button>

          {errorMessage ? (
            <div
              role="alert"
              className="flex min-h-11 items-start gap-3 rounded-card border border-danger/30 bg-panel-subtle p-3 text-body text-danger"
            >
              <AlertCircle size={18} aria-hidden="true" className="mt-0.5 shrink-0" />
              <p>{errorMessage}</p>
            </div>
          ) : null}
        </form>
      </section>
    </main>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { LoginPlate } from './LoginPlate';
import { MfaForm, ResetRequestForm } from './LoginForms';
import { LoginAlerts } from './LoginAlerts';
import { SignInOptions } from './SignInOptions';
import { LoginPasswordForm } from './LoginPasswordForm';
import { formatCountdown, useLockoutCountdown } from './useLockoutCountdown';
import { useAuth, type OIDCProvider } from './useAuth';
import { useBrandLogo } from '@resources/hooks/useBranding';
import { safeReturnTo } from './returnTo';

export default function LoginPage() {
  const { t } = useTranslation('auth');
  // `GET /branding` is public, so the install's own wordmark is available
  // here despite there being no session yet. The variant comes from the
  // persisted theme store, which is applied before this renders.
  const wordmark = useBrandLogo('wide');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [revealPassword, setRevealPassword] = useState(false);
  const [searchParams] = useSearchParams();
  const [error, setError] = useState(() => (searchParams.get('error') === 'oidc_failed' ? t('login.oidcError') : ''));
  const [loading, setLoading] = useState(false);
  const [lockoutUntil, setLockoutUntil] = useState<number | undefined>(undefined);
  // Non-null once the password cleared but a second factor is owed.
  // The challenge is server-side and expires on its own; holding the
  // id here is what swaps the form over to the code step.
  const [mfaChallengeId, setMfaChallengeId] = useState<string | null>(null);
  const [mfaCode, setMfaCode] = useState('');
  // "Forgot password" used to navigate to /reset-password, which threw away the
  // plate and the card to show one input. It is a third view of this card now.
  const [showReset, setShowReset] = useState(false);
  const [directoryId, setDirectoryId] = useState<string | null>(null);
  const login = useAuth((s) => s.login);
  const verifyMFA = useAuth((s) => s.verifyMFA);
  const oidcProviders = useAuth((s) => s.oidcProviders);
  const navigate = useNavigate();
  const landing = safeReturnTo(searchParams.get('next')) ?? '/';
  const { remaining: lockoutRemaining, done: lockoutDone } = useLockoutCountdown(lockoutUntil);

  useEffect(() => {
    if (searchParams.get('error') === 'oidc_failed') {
      toast.error(t('login.oidcError'));
    }
  }, [searchParams, t]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setLockoutUntil(undefined);
    setLoading(true);

    const result = directoryId ? await login(username, password, directoryId) : await login(username, password);
    setLoading(false);

    if (!result.success && result.mfaRequired && result.challengeId) {
      setMfaChallengeId(result.challengeId);
      setMfaCode('');
      setError('');
      return;
    }

    if (result.success) {
      // Forced password reset takes priority over the regular
      // dashboard nav — the user gets bumped to the change screen
      // before any protected route mounts.
      if ('mustResetPassword' in result && result.mustResetPassword) {
        void navigate('/force-password-change');
        return;
      }
      void navigate(landing, { replace: true });
      return;
    }

    const msg = result.error ?? t('login.failed');
    setError(msg);

    if (result.locked && result.retryAfterSeconds) {
      // Round up so the user never sees "0s" before the server
      // confirms the unlock — the server's lock table is the source
      // of truth, our countdown is a UX hint.
      setLockoutUntil(Math.max(1, Math.ceil(result.retryAfterSeconds)));
    } else {
      toast.error(msg);
    }
  }

  async function handleMFASubmit(e: FormEvent) {
    e.preventDefault();
    if (!mfaChallengeId) return;
    setError('');
    setLoading(true);

    const result = await verifyMFA(mfaChallengeId, mfaCode.trim());
    setLoading(false);

    if (result.success) {
      if ('mustResetPassword' in result && result.mustResetPassword) {
        void navigate('/force-password-change');
        return;
      }
      void navigate(landing, { replace: true });
      return;
    }

    // The store drops challengeId when the challenge itself is dead
    // (expired or already spent) — that can only be recovered by
    // starting the whole login again.
    if (!result.challengeId) {
      setMfaChallengeId(null);
      setMfaCode('');
      setPassword('');
      const expired = t('login.mfaExpired');
      setError(expired);
      toast.error(expired);
      return;
    }

    const msg = result.error || t('login.mfaFailed');
    setError(msg);
    toast.error(msg);
  }

  function resetToPasswordStep() {
    setMfaChallengeId(null);
    setMfaCode('');
    setPassword('');
    setError('');
  }

  function handleOIDCLogin(provider: OIDCProvider) {
    window.location.href = `/api/identity-providers/${provider.id}/authorize`;
  }

  function switchDirectory(id: string | null) {
    setDirectoryId(id);
    setPassword('');
    setError('');
  }

  const locked = !!lockoutUntil && !lockoutDone;
  const shownError = lockoutUntil && lockoutDone ? '' : error;
  const disabled = shownError.toLowerCase().includes('disabled');
  const submitDisabled = loading || locked;
  const countdown = formatCountdown(lockoutRemaining);
  const view: 'password' | 'mfa' | 'reset' = showReset ? 'reset' : mfaChallengeId ? 'mfa' : 'password';

  return (
    // The page itself never scrolls. Only the form column does, and only when
    // the card genuinely does not fit — a short window with the MFA step and an
    // error banner up at once.
    <div className="grid h-dvh overflow-hidden bg-background lg:grid-cols-[1.2fr_0.8fr]">
      <LoginPlate />

      <main className="flex items-center justify-center overflow-y-auto px-6 py-8">
        {/* The card lifts off the seam and sits over the plate on wide screens.
            `mc-glass-overlay` rather than `mc-glass`: this floats over a
            photograph, and --glass is 4.8% white in dark mode — opaque is the
            only readable substrate here. It owns its own shadow, so no
            `shadow-*` utility on top. */}
        <div className="mc-glass-overlay relative z-10 w-full max-w-sm rounded-2xl p-8 lg:-ml-[4.5rem]">
          {/* The page's only logo — the plate carries just its title block, so
              the mark does not swap sides at the `lg` breakpoint. */}
          <img
            src={wordmark.src}
            alt={t('brand.logoAlt', { ns: 'common' })}
            className="mx-auto mb-6 h-11 w-auto max-w-full object-contain"
          />

          <h1 className="mb-1.5 text-2xl font-bold tracking-tight text-foreground">
            {view === 'reset' ? t('passwordReset.requestTitle') : t('login.headline')}
          </h1>
          <p className="mb-6 text-sm text-muted-foreground">
            {view === 'reset' ? t('passwordReset.requestDescription') : t('login.title')}
          </p>

          {view !== 'reset' && (
            <LoginAlerts locked={locked} disabled={disabled} error={shownError} countdown={countdown} />
          )}

          {view === 'reset' ? (
            <ResetRequestForm onBack={() => setShowReset(false)} />
          ) : view === 'mfa' ? (
            <MfaForm
              code={mfaCode}
              onCodeChange={setMfaCode}
              loading={loading}
              onSubmit={handleMFASubmit}
              onBack={resetToPasswordStep}
            />
          ) : (
            <LoginPasswordForm
              onSubmit={handleSubmit}
              username={username}
              onUsernameChange={setUsername}
              password={password}
              onPasswordChange={setPassword}
              revealPassword={revealPassword}
              onToggleReveal={() => setRevealPassword((v) => !v)}
              onForgotPassword={() => setShowReset(true)}
              directoryId={directoryId}
              onDirectoryChange={setDirectoryId}
              providers={oidcProviders}
              locked={locked}
              loading={loading}
              submitDisabled={submitDisabled}
              countdown={countdown}
            />
          )}

          {view === 'password' && (
            <SignInOptions
              providers={oidcProviders}
              directoryId={directoryId}
              disabled={locked}
              onRedirect={handleOIDCLogin}
              onDirectory={switchDirectory}
              onLocal={() => switchDirectory(null)}
            />
          )}

          {/* Silent helper for screen-reader announcements — the
              visible countdown updates every second; the live region
              only fires when the gate flips state. */}
          <span aria-live="polite" className="sr-only">
            {locked ? t('login.lockedRetryIn', { time: countdown }) : ''}
          </span>
        </div>
      </main>
    </div>
  );
}

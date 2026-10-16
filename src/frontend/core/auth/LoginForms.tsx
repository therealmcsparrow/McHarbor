// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft } from '@tabler/icons-react';
import { api } from '@core/api/client';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useFocusOnMount } from '@resources/hooks/useFocusOnMount';

/** The two gate notices — lockout and disabled — share a shape. */
export function Notice({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <div
      role="alert"
      className="mb-4 flex items-start gap-3 rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
    >
      {icon}
      <div>
        <div className="font-medium">{title}</div>
        <div className="mt-1 text-destructive/90">{children}</div>
      </div>
    </div>
  );
}

/** The second step: the password cleared, a code is owed. */
export function MfaForm({
  code,
  onCodeChange,
  loading,
  onSubmit,
  onBack,
}: {
  code: string;
  onCodeChange: (value: string) => void;
  loading: boolean;
  onSubmit: (e: FormEvent) => void;
  onBack: () => void;
}) {
  const { t } = useTranslation('auth');
  const codeRef = useFocusOnMount<HTMLInputElement>();

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div>
        <p className="text-sm font-medium text-foreground">{t('login.mfaTitle')}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t('login.mfaHelp')}</p>
      </div>

      <div>
        <Label htmlFor="mfa-code" className="mb-1">
          {t('login.mfaLabel')}
        </Label>
        <Input
          variant="outline"
          id="mfa-code"
          type="text"
          value={code}
          onChange={(e) => onCodeChange(e.target.value)}
          required
          ref={codeRef}
          autoComplete="one-time-code"
          inputMode="numeric"
          maxLength={6}
          placeholder="123456"
          className="text-center font-mono text-lg tracking-[0.4em]"
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('login.mfaRecoveryHint')}</p>
      </div>

      <Button type="submit" disabled={loading || code.trim().length === 0} className="w-full">
        {loading ? t('login.mfaSubmitting') : t('login.mfaSubmit')}
      </Button>

      <Button type="button" variant="ghost" className="w-full" onClick={onBack}>
        {t('login.mfaBack')}
      </Button>
    </form>
  );
}

/**
 * "Forgot password", in place of the credential form rather than on a page of
 * its own.
 *
 * The neutrality of the response is the point and is preserved exactly from the
 * standalone page it replaces: `POST /api/auth/password/forgot` always renders
 * the same "if an account exists…" message, and a transport error is swallowed
 * so that a failure cannot be told apart from a success. Anything else turns
 * this form into an oracle for valid usernames.
 */
export function ResetRequestForm({ onBack }: { onBack: () => void }) {
  const { t } = useTranslation('auth');
  const identifierRef = useFocusOnMount<HTMLInputElement>();
  const [identifier, setIdentifier] = useState('');
  const [loading, setLoading] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!identifier.trim()) return;

    setLoading(true);
    try {
      // One field, either kind of value: the backend matches `identifier`
      // against both the username and the email column, so the form never has
      // to ask which one the account uses.
      await api.post('/auth/password/forgot', { identifier: identifier.trim() });
    } catch {
      // Swallowed deliberately — see the note above.
    }
    setLoading(false);
    setSubmitted(true);
  }

  return (
    <div className="space-y-4">
      {submitted ? (
        <output
          aria-live="polite"
          className="block rounded-lg border border-primary/40 bg-primary/10 p-4 text-sm text-foreground"
        >
          {t('passwordReset.requestSubmitted')}
        </output>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <Label htmlFor="identifier" className="mb-1">
              {t('passwordReset.identifier')}
            </Label>
            <Input
              variant="outline"
              id="identifier"
              type="text"
              autoComplete="username email"
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
              placeholder={t('passwordReset.identifierPlaceholder')}
              required
              ref={identifierRef}
            />
          </div>

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? t('passwordReset.submitting') : t('passwordReset.submit')}
          </Button>
        </form>
      )}

      <Button type="button" variant="ghost" className="w-full" onClick={onBack}>
        <IconArrowLeft className="size-4" aria-hidden="true" />
        {t('passwordReset.backToLogin')}
      </Button>
    </div>
  );
}

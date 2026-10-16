// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconLockSquare } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { api } from '@core/api/client';

/**
 * Public "Forgot password" form. Submits the identifier to
 *   POST /api/auth/password/forgot
 * The server always answers 200 with a neutral message — this page
 * renders that same message regardless of whether the account
 * exists, to avoid leaking valid usernames/emails.
 */
export default function PasswordResetRequestPage() {
  const { t } = useTranslation('auth');
  const [identifier, setIdentifier] = useState('');
  const [loading, setLoading] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!identifier.trim()) return;

    setLoading(true);
    try {
      // One field, either kind of value: the backend matches
      // `identifier` against both the username and the email column,
      // so the form never has to ask which one the account uses.
      await api.post('/auth/password/forgot', {
        identifier: identifier.trim(),
      });
    } catch {
      // Swallow errors — neutral response is rendered regardless.
      // The /forgot endpoint is designed never to leak account
      // existence, so any transport-level failure here is treated
      // the same as a normal submission.
    }
    setLoading(false);
    setSubmitted(true);
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
        <div className="mb-6 flex flex-col items-center text-center">
          <IconLockSquare className="mb-3 h-12 w-12 text-primary" aria-hidden="true" />
          <h1 className="text-xl font-semibold text-foreground">{t('passwordReset.requestTitle')}</h1>
          <p className="mt-2 text-sm text-muted-foreground">{t('passwordReset.requestDescription')}</p>
        </div>

        {submitted ? (
          <output
            aria-live="polite"
            className="block rounded-md border border-primary/40 bg-primary/10 p-4 text-sm text-foreground"
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
              />
            </div>

            <Button type="submit" disabled={loading} className="w-full">
              {loading ? t('passwordReset.submitting') : t('passwordReset.submit')}
            </Button>
          </form>
        )}

        <div className="mt-6 flex justify-center">
          <Link
            to="/login"
            className="inline-flex items-center gap-1 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
            {t('passwordReset.backToLogin')}
          </Link>
        </div>
      </div>
    </div>
  );
}

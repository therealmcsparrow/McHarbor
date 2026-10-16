// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconArrowLeft, IconLockSquare } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { api } from '@core/api/client';
import { usePasswordPolicy } from './usePasswordPolicy';
import { isPasswordValid } from './passwordPolicy';

/**
 * Consume a password-reset token: this page is reached from a link
 * in the reset email (`/reset-password/:token`). On success the
 * user is redirected to /login — they must re-authenticate because
 * the reset endpoint forcibly revokes all existing sessions.
 */
export default function PasswordResetConsumePage() {
  const { t } = useTranslation('auth');
  const { t: tc } = useTranslation('common');
  const { policy } = usePasswordPolicy();
  const { token = '' } = useParams<{ token: string }>();
  const navigate = useNavigate();
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');

    if (newPassword !== confirmPassword) {
      setError(t('passwordReset.passwordMismatch'));
      return;
    }
    if (!isPasswordValid(newPassword, policy)) {
      setError(tc('password.policyNotMet'));
      return;
    }

    setLoading(true);
    // api.post resolves with { success: false } on a 4xx — it does not
    // throw — so the result has to be inspected. The previous bare
    // try/catch reported success for every rejected password.
    const res = await api.post('/auth/password/reset', {
      token,
      newPassword,
    });
    setLoading(false);

    if (res.success) {
      toast.success(t('passwordReset.success'));
      void navigate('/login');
      return;
    }
    // The server localises policy rejections ("too short", "reused");
    // show its message rather than a generic failure.
    const msg = res.error ? t('passwordReset.failed', { error: res.error }) : t('passwordReset.failedGeneric');
    setError(msg);
    toast.error(msg);
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
        <div className="mb-6 flex flex-col items-center text-center">
          <IconLockSquare className="mb-3 h-12 w-12 text-primary" aria-hidden="true" />
          <h1 className="text-xl font-semibold text-foreground">{t('passwordReset.consumeTitle')}</h1>
          <p className="mt-2 text-sm text-muted-foreground">{t('passwordReset.consumeDescription')}</p>
        </div>

        {error && (
          <div
            role="alert"
            className="mb-4 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive"
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <Label htmlFor="newPassword" className="mb-1">
              {t('passwordReset.newPassword')}
            </Label>
            <Input
              variant="outline"
              id="newPassword"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              aria-describedby="consume-password-requirements"
            />
            <PasswordRequirements id="consume-password-requirements" password={newPassword} policy={policy} />
          </div>

          <div>
            <Label htmlFor="confirmPassword" className="mb-1">
              {t('passwordReset.confirmPassword')}
            </Label>
            <Input
              variant="outline"
              id="confirmPassword"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
            />
          </div>

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? t('passwordReset.submitting') : t('passwordReset.submit')}
          </Button>
        </form>

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

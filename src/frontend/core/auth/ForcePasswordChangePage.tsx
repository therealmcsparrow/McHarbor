// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconLockExclamation } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useFocusOnMount } from '@resources/hooks/useFocusOnMount';
import { Text as UiText, Caption } from '@resources/components/ui/Typography';
import { Box, Flex } from '@resources/components/ui/Layout';
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { api } from '@core/api/client';
import { useAuth } from './useAuth';
import { usePasswordPolicy } from './usePasswordPolicy';
import { isPasswordValid } from './passwordPolicy';

type ForcePasswordChangePageProps = {
  /**
   * When true, the page is rendered as a full-page screen instead
   * of an inline card. LoginPage routes here when the login
   * response carries `mustResetPassword: true`.
   */
  fullScreen?: boolean;
};

/**
 * ForcePasswordChangePage is shown to a user whose account requires
 * a password change (`must_reset_password=1`). Flow:
 *
 *   1. User just authenticated. LoginPage stores the typed password
 *      in the auth store as `lastLoginPassword` so we can satisfy
 *      the backend's "current password" check without prompting.
 *   2. They enter a new password + confirm.
 *   3. PUT /users/{me}/password with both fields; the server clears
 *      `must_reset_password=0` and `password_failed_count=0`.
 *   4. We clear `mustResetPassword` + `lastLoginPassword` in the
 *      store and route to /dashboard.
 *
 * Sign-out is the only escape hatch if the user refuses.
 */
export default function ForcePasswordChangePage({ fullScreen = true }: ForcePasswordChangePageProps) {
  const { t } = useTranslation('auth');
  const { t: tc } = useTranslation('common');
  const passwordRef = useFocusOnMount<HTMLInputElement>();
  const { policy } = usePasswordPolicy();
  const navigate = useNavigate();
  const user = useAuth((s) => s.user);
  const mustResetPassword = useAuth((s) => s.mustResetPassword);
  const consumeLastLoginPassword = useAuth((s) => s.consumeLastLoginPassword);
  const clearMustResetPassword = useAuth((s) => s.clearMustResetPassword);
  const logout = useAuth((s) => s.logout);
  const [password, setPassword] = useState('');
  const [confirmation, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  // Route guard. If we somehow land here without an active session
  // (e.g. the user pasted the URL into a fresh tab), bounce them
  // back to login. If the session is active but `mustResetPassword`
  // is false, the screen is stale — push to dashboard.
  useEffect(() => {
    if (!user) {
      void navigate('/login', { replace: true });
      return;
    }
    if (!mustResetPassword) {
      void navigate('/dashboard', { replace: true });
    }
  }, [user, mustResetPassword, navigate]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');

    if (password !== confirmation) {
      setError(t('forceChange.passwordMismatch'));
      return;
    }
    if (!isPasswordValid(password, policy)) {
      setError(tc('password.policyNotMet'));
      return;
    }

    if (!user) {
      void navigate('/login');
      return;
    }

    const currentPassword = consumeLastLoginPassword();
    if (!currentPassword) {
      // We lost the in-memory password (refresh, second tab).
      // Sign out so the user starts fresh — they have to type
      // both the current and new password on the next login.
      logout();
      return;
    }

    setLoading(true);
    const res = await api.put(`/users/${user.id}/password`, {
      newPassword: password,
      currentPassword,
    });
    setLoading(false);

    if (res.success) {
      toast.success(t('forceChange.success'));
      // Refresh the cached user so the server-side flag is fresh.
      await useAuth.getState().checkSession();
      // Belt-and-suspenders: ensure local state is cleared even
      // if checkSession races with a backend hiccup.
      clearMustResetPassword();
      void navigate('/dashboard', { replace: true });
      return;
    }
    // The likeliest rejection here is "reused": this screen exists
    // because an admin forced a change, and the obvious move is to
    // retype the old password. The server says so — repeat it.
    const msg = res.error || t('forceChange.failed');
    setError(msg);
    toast.error(msg);
  }

  function handleSignOut() {
    clearMustResetPassword();
    logout();
    void navigate('/login');
  }

  const card = (
    <Box className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
      <Flex direction="col" align="center" gap={3} className="mb-6">
        <Flex align="center" justify="center" aria-hidden="true" className="size-12 bg-warning/10 text-warning">
          <IconLockExclamation className="size-5" />
        </Flex>
        <Caption>{t('forceChange.title')}</Caption>
      </Flex>
      <UiText size="sm" tone="muted" className="mb-6">
        {t('forceChange.description')}
      </UiText>
      {error && (
        <Box className="mb-4 rounded-md border border-destructive/50 bg-destructive/10 p-3">
          <UiText size="sm" tone="destructive">
            {error}
          </UiText>
        </Box>
      )}
      <form onSubmit={handleSubmit} className="space-y-4">
        <Box>
          <Label htmlFor="new" className="mb-1">
            {t('forceChange.newPassword')}
          </Label>
          <Input
            variant="outline"
            id="new"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            ref={passwordRef}
            aria-describedby="force-password-requirements"
          />
          <PasswordRequirements id="force-password-requirements" password={password} policy={policy} />
        </Box>
        <Box>
          <Label htmlFor="confirm" className="mb-1">
            {t('forceChange.confirmPassword')}
          </Label>
          <Input
            variant="outline"
            id="confirm"
            type="password"
            value={confirmation}
            onChange={(e) => setConfirm(e.target.value)}
            required
          />
        </Box>
        <Button type="submit" disabled={loading} className="w-full">
          {loading ? t('forceChange.submitting') : t('forceChange.submit')}
        </Button>
        <Button type="button" variant="ghost" size="sm" className="w-full" onClick={handleSignOut}>
          {t('forceChange.signOutInstead')}
        </Button>
      </form>
    </Box>
  );

  if (!fullScreen) return card;
  return (
    <Flex align="center" justify="center" className="min-h-screen bg-background">
      {card}
    </Flex>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { Navigate, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { PasswordRequirements } from '@resources/components/ui/PasswordRequirements';
import { useAuth } from './useAuth';
import { usePasswordPolicy } from './usePasswordPolicy';
import { isPasswordValid } from './passwordPolicy';
import { useDatabaseSetupStatus } from './useDatabaseSetupStatus';
import { SetupTokenField } from './SetupTokenField';

export default function SetupPage() {
  const { t } = useTranslation('auth');
  const { t: tc } = useTranslation('common');
  const { policy } = usePasswordPolicy();
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const setup = useAuth((s) => s.setup);
  const navigate = useNavigate();
  // The first admin is the first row McHarbor writes, so it must not be
  // written before the operator has said which database it goes into.
  // Anyone who reaches this page directly is sent back one step.
  const dbStatus = useDatabaseSetupStatus();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError(t('setup.passwordMismatch'));
      return;
    }

    // The first admin is held to the same policy as everyone else —
    // the backend enforces it in HandleSetup either way.
    if (!isPasswordValid(password, policy)) {
      setError(tc('password.policyNotMet'));
      return;
    }

    setLoading(true);
    const result = await setup(username, password, email || undefined);
    setLoading(false);

    if (result.success) {
      void navigate('/dashboard');
    } else {
      const msg = result.error ?? t('setup.failed');
      setError(msg);
      toast.error(msg);
    }
  }

  if (dbStatus.isLoading) return null;
  if (dbStatus.data && !dbStatus.data.configured && !dbStatus.data.envPinned) {
    return <Navigate to="/setup/database" replace />;
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
        <div className="mb-6 flex flex-col items-center">
          <img src="/logo_McSparrow.svg" alt={t('brand.logoAlt', { ns: 'common' })} className="mb-3 h-12" />
          <p className="text-sm text-muted-foreground">{t('setup.title')}</p>
        </div>

        {error && (
          <div className="mb-4 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <SetupTokenField />
          <div>
            <Label htmlFor="username" className="mb-1">
              {t('setup.username')}
            </Label>
            <Input
              variant="outline"
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
            />
          </div>

          <div>
            <Label htmlFor="email" className="mb-1">
              {t('setup.email')}
            </Label>
            <Input variant="outline" id="email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>

          <div>
            <Label htmlFor="password" className="mb-1">
              {t('setup.password')}
            </Label>
            <Input
              variant="outline"
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              aria-describedby="setup-password-requirements"
            />
            <PasswordRequirements
              id="setup-password-requirements"
              showHistory={false}
              password={password}
              policy={policy}
            />
          </div>

          <div>
            <Label htmlFor="confirmPassword" className="mb-1">
              {t('setup.confirmPassword')}
            </Label>
            <Input
              variant="outline"
              id="confirmPassword"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
            />
          </div>

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? t('setup.submitting') : t('setup.submit')}
          </Button>
        </form>
      </div>
    </div>
  );
}

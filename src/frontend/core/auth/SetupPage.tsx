// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useAuth } from './useAuth';

export default function SetupPage() {
  const { t } = useTranslation('auth');
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const setup = useAuth((s) => s.setup);
  const navigate = useNavigate();

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError(t('setup.passwordMismatch'));
      return;
    }

    if (password.length < 8) {
      setError(t('setup.passwordShort'));
      return;
    }

    setLoading(true);
    const result = await setup(username, password, email || undefined);
    setLoading(false);

    if (result.success) {
      navigate('/dashboard');
    } else {
      const msg = result.error ?? t('setup.failed');
      setError(msg);
      toast.error(msg);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
        <div className="mb-6 flex flex-col items-center">
          <img src="/logo_McSparrow.svg" alt="McHarbor" className="mb-3 h-12" />
          <p className="text-sm text-muted-foreground">
            {t('setup.title')}
          </p>
        </div>

        {error && (
          <div className="mb-4 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <Label htmlFor="username" className="mb-1">{t('setup.username')}</Label>
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
            <Label htmlFor="email" className="mb-1">{t('setup.email')}</Label>
            <Input
              variant="outline"
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>

          <div>
            <Label htmlFor="password" className="mb-1">{t('setup.password')}</Label>
            <Input
              variant="outline"
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              minLength={8}
            />
          </div>

          <div>
            <Label htmlFor="confirmPassword" className="mb-1">{t('setup.confirmPassword')}</Label>
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

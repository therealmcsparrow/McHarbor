// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect, type FormEvent } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { IconBrandAzure, IconBrandGoogle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useAuth, type OIDCProvider } from './useAuth';

const PROVIDER_ICONS: Record<string, typeof IconBrandAzure> = {
  entra_id: IconBrandAzure,
  google: IconBrandGoogle,
};

export default function LoginPage() {
  const { t } = useTranslation('auth');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const login = useAuth((s) => s.login);
  const oidcProviders = useAuth((s) => s.oidcProviders);
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  useEffect(() => {
    if (searchParams.get('error') === 'oidc_failed') {
      const msg = t('login.oidcError');
      setError(msg);
      toast.error(msg);
    }
  }, [searchParams, t]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);

    const result = await login(username, password);
    setLoading(false);

    if (result.success) {
      navigate('/dashboard');
    } else {
      const msg = result.error ?? t('login.failed');
      setError(msg);
      toast.error(msg);
    }
  }

  function handleOIDCLogin(provider: OIDCProvider) {
    window.location.href = `/api/identity-providers/${provider.id}/authorize`;
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border border-border bg-card p-8 shadow-lg">
        <div className="mb-6 flex flex-col items-center">
          <img src="/logo_McSparrow.svg" alt="McHarbor" className="mb-3 h-12" />
          <p className="text-sm text-muted-foreground">
            {t('login.title')}
          </p>
        </div>

        {error && (
          <div className="mb-4 rounded-md border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <Label htmlFor="username" className="mb-1">
              {t('login.username')}
            </Label>
            <Input
              variant="outline"
              id="username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              autoFocus
              placeholder={t('login.usernamePlaceholder')}
            />
          </div>

          <div>
            <Label htmlFor="password" className="mb-1">
              {t('login.password')}
            </Label>
            <Input
              variant="outline"
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>

          <Button type="submit" disabled={loading} className="w-full">
            {loading ? t('login.submitting') : t('login.submit')}
          </Button>
        </form>

        {oidcProviders.length > 0 && (
          <>
            <div className="my-4 flex items-center gap-3">
              <div className="h-px flex-1 bg-border" />
              <span className="text-xs text-muted-foreground">{t('login.orSeparator')}</span>
              <div className="h-px flex-1 bg-border" />
            </div>

            <div className="space-y-2">
              {oidcProviders.map((provider) => {
                const Icon = PROVIDER_ICONS[provider.providerType];
                return (
                  <Button
                    key={provider.id}
                    variant="outline"
                    className="w-full"
                    onClick={() => handleOIDCLogin(provider)}
                  >
                    {Icon && <Icon className="size-4" />}
                    {t('login.signInWith', { provider: provider.name })}
                  </Button>
                );
              })}
            </div>
          </>
        )}
      </div>
    </div>
  );
}

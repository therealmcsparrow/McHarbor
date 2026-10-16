// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Button } from '@resources/components/ui/Button';
import { useFocusOnMount } from '@resources/hooks/useFocusOnMount';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { DirectoryPicker } from './SignInOptions';
import type { OIDCProvider } from './authTypes';

type LoginPasswordFormProps = {
  onSubmit: (e: FormEvent) => void;
  username: string;
  onUsernameChange: (value: string) => void;
  password: string;
  onPasswordChange: (value: string) => void;
  revealPassword: boolean;
  onToggleReveal: () => void;
  onForgotPassword: () => void;
  directoryId: string | null;
  onDirectoryChange: (id: string | null) => void;
  providers: OIDCProvider[];
  locked: boolean;
  loading: boolean;
  submitDisabled: boolean;
  countdown: string;
};

export function LoginPasswordForm({
  onSubmit,
  username,
  onUsernameChange,
  password,
  onPasswordChange,
  revealPassword,
  onToggleReveal,
  onForgotPassword,
  directoryId,
  onDirectoryChange,
  providers,
  locked,
  loading,
  submitDisabled,
  countdown,
}: LoginPasswordFormProps) {
  const { t } = useTranslation('auth');
  const usernameRef = useFocusOnMount<HTMLInputElement>();
  return (
    <form onSubmit={onSubmit} className="space-y-4">
      {directoryId && (
        <DirectoryPicker providers={providers} value={directoryId} onChange={onDirectoryChange} disabled={locked} />
      )}
      <div>
        <Label htmlFor="username" className="mb-1">
          {t('login.username')}
        </Label>
        <Input
          variant="outline"
          id="username"
          type="text"
          value={username}
          onChange={(e) => onUsernameChange(e.target.value)}
          required
          ref={usernameRef}
          disabled={locked}
          autoComplete="username"
          placeholder={t('login.usernamePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="password" className="mb-1">
          {t('login.password')}
        </Label>
        <div className="relative">
          <Input
            variant="outline"
            id="password"
            type={revealPassword ? 'text' : 'password'}
            value={password}
            onChange={(e) => onPasswordChange(e.target.value)}
            required
            disabled={locked}
            autoComplete="current-password"
            className="pr-11"
          />
          <PlainButton
            onClick={onToggleReveal}
            disabled={locked}
            aria-label={revealPassword ? t('login.hidePassword') : t('login.showPassword')}
            className="absolute top-1/2 right-1.5 grid size-8 -translate-y-1/2 place-items-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            {revealPassword ? (
              <IconEyeOff className="size-4" aria-hidden="true" />
            ) : (
              <IconEye className="size-4" aria-hidden="true" />
            )}
          </PlainButton>
        </div>
        {!directoryId && (
          <div className="mt-1.5 flex justify-end">
            <PlainButton
              onClick={onForgotPassword}
              className="rounded text-xs font-medium text-primary hover:underline"
            >
              {t('login.forgotPassword')}
            </PlainButton>
          </div>
        )}
      </div>

      <Button type="submit" disabled={submitDisabled} className="w-full">
        {loading ? t('login.submitting') : locked ? t('login.lockedRetryIn', { time: countdown }) : t('login.submit')}
      </Button>
    </form>
  );
}

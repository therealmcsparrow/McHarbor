// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconKey, IconPalette, IconShieldLock, IconUserEdit } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { languageLabels } from '@core/i18n';
import { useLanguageStore } from '@resources/stores/language';
import { useTheme } from '@resources/hooks/useTheme';
import type { ProfileTabId } from '../ProfileTabs';

type OverviewTabProps = {
  username: string;
  displayName: string | null;
  email: string | null;
  identityProviderLocked: boolean;
  systemAccount: boolean;
  mfaEnabled?: boolean;
  onNavigate: (tab: ProfileTabId) => void;
};

export function OverviewTab({
  username,
  displayName,
  email,
  identityProviderLocked,
  systemAccount,
  mfaEnabled,
  onNavigate,
}: OverviewTabProps) {
  const { t } = useTranslation('common');
  const { theme } = useTheme();
  const language = useLanguageStore((s) => s.language);

  // A static map rather than an interpolated key: the i18n CI gate reads
  // literal `t('...')` calls, and a template literal is invisible to it.
  const themeLabels: Record<typeof theme, string> = {
    light: t('theme.light'),
    dark: t('theme.dark'),
    system: t('theme.system'),
  };

  // The container overview offers whatever the container's state makes
  // useful next. The account equivalent: an unedited local account wants
  // its details filled in, an account without a second factor wants one.
  const suggestedActions = [
    {
      key: 'account',
      icon: IconUserEdit,
      label: t('profile.account.editTitle'),
      onClick: () => onNavigate('account'),
      disabled: systemAccount || identityProviderLocked,
      primary: !systemAccount && !identityProviderLocked && !email,
    },
    {
      key: 'security',
      icon: IconShieldLock,
      label: t('profile.security.mfaTitle'),
      onClick: () => onNavigate('security'),
      disabled: systemAccount,
      primary: !systemAccount && mfaEnabled === false,
    },
    {
      key: 'password',
      icon: IconKey,
      label: t('profile.account.password.title'),
      onClick: () => onNavigate('security'),
      disabled: systemAccount || identityProviderLocked,
      primary: false,
    },
    {
      key: 'appearance',
      icon: IconPalette,
      label: t('profile.appearance.title'),
      onClick: () => onNavigate('appearance'),
      disabled: false,
      primary: false,
    },
  ];

  return (
    <div className="space-y-6">
      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.overview.quickActions')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.overview.quickActionsDescription')}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          {suggestedActions.map((action) => {
            const Icon = action.icon;
            return (
              <Button
                key={action.key}
                type="button"
                variant={action.primary ? 'default' : 'outline'}
                size="sm"
                className="gap-1.5"
                onClick={action.onClick}
                disabled={action.disabled}
              >
                <Icon className="size-4" />
                {action.label}
              </Button>
            );
          })}
        </div>
      </section>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.overview.identity')}
          </h3>
          <InfoRow label={t('profile.account.username')}>
            <span className="font-mono text-xs">{username}</span>
          </InfoRow>
          <InfoRow label={t('profile.account.displayName')}>{displayName || '-'}</InfoRow>
          <InfoRow label={t('profile.account.email')}>{email || t('profile.account.noEmail')}</InfoRow>
          <InfoRow label={t('profile.overview.accountType')}>
            <StatusIndicator
              tone={systemAccount ? 'neutral' : 'success'}
              label={
                systemAccount
                  ? t('profile.badges.system')
                  : identityProviderLocked
                    ? t('profile.badges.idp')
                    : t('profile.badges.local')
              }
              size="sm"
            />
          </InfoRow>
          <InfoRow label={t('profile.security.mfaTitle')}>
            <StatusIndicator
              tone={mfaEnabled ? 'success' : 'neutral'}
              label={mfaEnabled ? t('profile.badges.mfaOn') : t('profile.badges.mfaOff')}
              size="sm"
            />
          </InfoRow>
        </div>

        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('profile.overview.preferences')}
          </h3>
          <InfoRow label={t('theme.label')}>{themeLabels[theme]}</InfoRow>
          <InfoRow label={t('language.label')}>{languageLabels[language]}</InfoRow>
          <p className="mt-4 text-xs text-muted-foreground">{t('profile.overview.preferencesDescription')}</p>
        </div>
      </div>
    </div>
  );
}

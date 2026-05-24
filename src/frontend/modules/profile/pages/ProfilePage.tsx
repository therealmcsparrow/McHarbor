// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  IconCheck,
  IconDeviceDesktop,
  IconLanguage,
  IconMoon,
  IconSun,
  IconUserCircle,
  type TablerIcon,
} from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import { useTheme } from '@resources/hooks/useTheme';
import { useLanguageStore } from '@resources/stores/language';
import { useAuth } from '@core/auth/useAuth';
import { languageLabels, supportedLanguages, type SupportedLanguage } from '@core/i18n';
import { cn } from '@resources/utils/cn';

type ThemeOption = {
  value: 'light' | 'dark' | 'system';
  label: string;
  description: string;
  icon: TablerIcon;
};

export default function ProfilePage() {
  const { t } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const { theme, setTheme } = useTheme();
  const language = useLanguageStore((s) => s.language);
  const setLanguage = useLanguageStore((s) => s.setLanguage);

  const themeOptions: ThemeOption[] = [
    {
      value: 'light',
      label: t('theme.light'),
      description: t('profile.preferences.themeLightDescription'),
      icon: IconSun,
    },
    {
      value: 'dark',
      label: t('theme.dark'),
      description: t('profile.preferences.themeDarkDescription'),
      icon: IconMoon,
    },
    {
      value: 'system',
      label: t('theme.system'),
      description: t('profile.preferences.themeSystemDescription'),
      icon: IconDeviceDesktop,
    },
  ];

  const displayName = user?.displayName || user?.username || t('profile.account.unknownUser');
  const initials = displayName.charAt(0).toUpperCase();

  return (
    <div className="space-y-6">
      <PageHeader title={t('profile.title')} description={t('profile.description')} />

      <div className="grid gap-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.4fr)]">
        <section className="rounded-lg border border-border bg-card p-6">
          <div className="flex items-center gap-4">
            <div className="flex size-14 items-center justify-center rounded-full bg-primary text-lg font-semibold text-primary-foreground">
              {initials}
            </div>
            <div className="min-w-0">
              <h2 className="truncate text-base font-semibold text-foreground">{displayName}</h2>
              <p className="truncate text-sm text-muted-foreground">{user?.email || t('profile.account.noEmail')}</p>
            </div>
          </div>

          <div className="mt-6 space-y-4 border-t border-border pt-5">
            <InfoRow label={t('profile.account.username')} value={user?.username ?? '-'} />
            <InfoRow label={t('profile.account.displayName')} value={user?.displayName || '-'} />
            <InfoRow label={t('profile.account.email')} value={user?.email || '-'} />
          </div>
        </section>

        <section className="rounded-lg border border-border bg-card p-6">
          <div className="flex items-start gap-3">
            <IconUserCircle className="mt-0.5 size-5 text-primary" />
            <div>
              <h2 className="text-base font-semibold text-foreground">{t('profile.preferences.title')}</h2>
              <p className="mt-1 text-sm text-muted-foreground">{t('profile.preferences.description')}</p>
            </div>
          </div>

          <div className="mt-6 space-y-6">
            <div>
              <div className="mb-3 flex items-center gap-2">
                <IconDeviceDesktop className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-medium text-foreground">{t('theme.label')}</h3>
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                {themeOptions.map((option) => (
                  <Button
                    key={option.value}
                    type="button"
                    variant="outline"
                    onClick={() => setTheme(option.value)}
                    className={cn(
                      'h-auto justify-start p-4 text-left',
                      theme === option.value && 'border-primary bg-primary/10 text-primary',
                    )}
                  >
                    <option.icon className="size-5 shrink-0" />
                    <span className="min-w-0">
                      <span className="flex items-center gap-2 text-sm font-medium">
                        {option.label}
                        {theme === option.value && <IconCheck className="size-4" />}
                      </span>
                      <span className="mt-1 block whitespace-normal text-xs font-normal text-muted-foreground">
                        {option.description}
                      </span>
                    </span>
                  </Button>
                ))}
              </div>
            </div>

            <div>
              <div className="mb-3 flex items-center gap-2">
                <IconLanguage className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-medium text-foreground">{t('language.label')}</h3>
              </div>
              <div className="max-w-sm">
                <Select
                  value={language}
                  onChange={(value) => setLanguage(value as SupportedLanguage)}
                  options={supportedLanguages.map((lang) => ({
                    value: lang,
                    label: languageLabels[lang],
                  }))}
                />
              </div>
              <p className="mt-2 text-xs text-muted-foreground">{t('profile.preferences.languageDescription')}</p>
            </div>
          </div>
        </section>
      </div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs font-medium uppercase text-muted-foreground">{label}</p>
      <p className="mt-1 break-words text-sm text-foreground">{value}</p>
    </div>
  );
}

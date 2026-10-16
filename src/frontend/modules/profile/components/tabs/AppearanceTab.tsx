// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconDeviceDesktop, IconMoon, IconSun, type TablerIcon } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { Button } from '@resources/components/ui/Button';
import { useTheme } from '@resources/hooks/useTheme';
import { useLanguageStore } from '@resources/stores/language';
import { useAuth } from '@core/auth/useAuth';
import { languageLabels, supportedLanguages, type SupportedLanguage } from '@core/i18n';
import { cn } from '@resources/utils/cn';
import { AppearancePreferencesCard } from './AppearancePreferencesCard';

type ThemeOption = {
  value: 'light' | 'dark' | 'system';
  label: string;
  description: string;
  icon: TablerIcon;
};

export function AppearanceTab() {
  const { t } = useTranslation('common');
  const { theme, setTheme } = useTheme();
  const language = useLanguageStore((s) => s.language);
  const updatePreferences = useAuth((s) => s.updatePreferences);

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

  return (
    <div className="space-y-6">
      {/* Theme, accent, font and base font size. Governed: this account may
          only override them while the operator allows it, and the card says so
          when they cannot. Light/dark below stays unconditional - it is a
          reading-comfort switch, not a brand choice, and the topbar toggle
          would contradict a lock on it anyway. */}
      <AppearancePreferencesCard className="mc-glass" />

      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">{t('theme.label')}</h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.preferences.description')}</p>
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
                <span className="mt-1 block text-xs font-normal whitespace-normal text-muted-foreground">
                  {option.description}
                </span>
              </span>
            </Button>
          ))}
        </div>
      </section>

      <section className="mc-glass rounded-lg p-6">
        <div className="mb-4">
          <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('language.label')}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">{t('profile.preferences.languageDescription')}</p>
        </div>
        <div className="max-w-sm">
          <Select
            value={language}
            onChange={(value) => {
              void updatePreferences({
                preferredLanguage: value as SupportedLanguage,
              });
            }}
            options={supportedLanguages.map((lang) => ({
              value: lang,
              label: languageLabels[lang],
            }))}
          />
        </div>
      </section>
    </div>
  );
}

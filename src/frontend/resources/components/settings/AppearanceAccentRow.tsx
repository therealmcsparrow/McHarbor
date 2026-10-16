// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';
import { ACCENT_CHOICES, ACCENT_TOKENS, type ThemeAccent } from '@resources/stores/themeCustomizer';

type AppearanceAccentRowProps = {
  accent: ThemeAccent;
  onPick: (a: ThemeAccent) => void;
  disabled?: boolean;
  permission?: string;
  idPrefix: string;
};

/**
 * The accent strip. The leading button hands `--primary` back to the theme;
 * the rest override it, and the aurora, the accent wash and the brand
 * gradient follow because they are derived from it.
 */
export function AppearanceAccentRow({ accent, onPick, disabled, permission, idPrefix }: AppearanceAccentRowProps) {
  const { t } = useTranslation('settings');
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        id={`${idPrefix}-accent-theme`}
        type="button"
        permission={permission}
        variant={accent === 'theme' ? 'default' : 'outline'}
        size="sm"
        disabled={disabled}
        onClick={() => onPick('theme')}
      >
        {t('appearance.accentTheme')}
      </Button>
      {ACCENT_CHOICES.map((accentName) => (
        <Button
          key={accentName}
          type="button"
          permission={permission}
          variant="outline"
          size="icon"
          disabled={disabled}
          aria-label={t(`appearance.accent.${accentName}`)}
          aria-pressed={accent === accentName}
          onClick={() => onPick(accentName)}
          className={cn('relative', accent === accentName && 'ring-2 ring-ring ring-offset-2 ring-offset-background')}
        >
          <span
            className="size-4 rounded-full border border-hairline bg-(--swatch)"
            // A generated colour from the accent table — no token exists for
            // "the swatch of the accent this button offers".
            style={cssVars({ swatch: `oklch(${ACCENT_TOKENS[accentName].primary})` })}
          />
        </Button>
      ))}
    </div>
  );
}

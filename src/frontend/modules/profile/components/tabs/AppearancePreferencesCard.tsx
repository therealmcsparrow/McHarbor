// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// i18n keys added by this file:
//   settings:appearance.{title,description,useOwnLabel,useOwnDescription}
//   settings:appearance.{overrideBlockedTitle,overrideBlockedBody}

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPalette } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { useResolvedAppearance } from '@resources/hooks/useResolvedAppearance';
import { useAuth } from '@core/auth/useAuth';
import { useUpdatePreferences } from '@core/auth/usePreferences';
import { AppearanceKnobFields, type AppearanceKnobs } from '@resources/components/settings/AppearanceKnobFields';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';

type AppearancePreferencesCardProps = {
  /** Surface material, so the card can sit on the bare profile canvas. */
  className?: string;
};

/**
 * This account's own theme, accent, interface font and base font size.
 *
 * The card has two states and the switch at the top decides which. Off, the
 * account follows whatever the operator set and keeps following it when the
 * operator changes it later. On, these four values apply instead.
 *
 * When the operator has disallowed overrides the switch is disabled and the
 * card explains why, rather than disappearing: a user looking at a theme they
 * did not choose should be able to find out that the choice was made for
 * them. Exactly the arrangement `NotificationsCard` uses for toasts.
 */
export function AppearancePreferencesCard({ className }: AppearancePreferencesCardProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const updatePreferences = useUpdatePreferences(t('general.preferencesSaveFailed'));
  const resolved = useResolvedAppearance();

  // `overridden` is the server's answer, `useOwn` the form's. They start equal
  // and diverge while the user is deciding.
  const [useOwn, setUseOwn] = useState(resolved.overridden);
  const [draft, setDraft] = useState<AppearanceKnobs>({
    themeId: resolved.themeId,
    accent: resolved.accent,
    font: resolved.font,
    baseFontPx: resolved.baseFontPx,
  });

  // Re-seed from the resolved values whenever the server's answer changes — a
  // save here, or an operator's reset landing through the session query.
  // Turning the switch on therefore starts from the current default rather
  // than from a blank form, which is what "customize this" should mean.
  const resolvedKey = [resolved.overridden, resolved.themeId, resolved.accent, resolved.font, resolved.baseFontPx].join(
    '|',
  );
  const [seededKey, setSeededKey] = useState(resolvedKey);
  if (seededKey !== resolvedKey) {
    setSeededKey(resolvedKey);
    setUseOwn(resolved.overridden);
    setDraft({
      themeId: resolved.themeId,
      accent: resolved.accent,
      font: resolved.font,
      baseFontPx: resolved.baseFontPx,
    });
  }

  async function handleSave() {
    try {
      await updatePreferences.mutateAsync(
        useOwn
          ? {
              appearanceThemeId: draft.themeId,
              appearanceAccent: draft.accent,
              appearanceFont: draft.font,
              appearanceBaseFontPx: draft.baseFontPx,
            }
          : { clearAppearanceOverride: true },
      );
      toast.success(t('general.preferencesSaved'));
    } catch {
      // The preferences mutation reports its own failure through the global mutation cache.
    }
  }

  if (!user) return null;

  return (
    <SettingsGroup
      icon={IconPalette}
      title={t('appearance.title')}
      description={t('appearance.description')}
      scope={t('general.scopePersonal')}
      className={className}
      actions={
        <Button onClick={handleSave} disabled={updatePreferences.isPending || !resolved.allowUserOverride}>
          {updatePreferences.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      {!resolved.allowUserOverride && (
        <Alert
          variant="info"
          title={t('appearance.overrideBlockedTitle')}
          description={t('appearance.overrideBlockedBody')}
        />
      )}

      <SettingsField
        htmlFor="appearance-use-own"
        label={t('appearance.useOwnLabel')}
        description={t('appearance.useOwnDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <Switch
          id="appearance-use-own"
          checked={useOwn}
          onCheckedChange={setUseOwn}
          disabled={!resolved.allowUserOverride}
          aria-label={t('appearance.useOwnLabel')}
        />
      </SettingsField>

      <AppearanceKnobFields
        value={draft}
        onChange={setDraft}
        disabled={!useOwn || !resolved.allowUserOverride}
        idPrefix="appearance-user"
        // Back to the operator's number, not to 16 — this card overrides the
        // app-wide layer, so that layer is what "reset" means here.
        baseFontDefault={resolved.defaults.baseFontPx}
      />
    </SettingsGroup>
  );
}

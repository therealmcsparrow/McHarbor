// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// i18n keys added by this file:
//   settings:appearance.{defaultsTitle,defaultsDescription,readOnlyTitle}
//   settings:appearance.{personalHint,personalHintLink}
//   settings:appearance.{allowOverrideLabel,allowOverrideDescription}
//   settings:appearance.{resetAction,resetHint,resetTitle,resetDescription}

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconPalette, IconRotate } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { APPEARANCE_FALLBACK } from '@resources/hooks/useResolvedAppearance';
import { PERM } from '@core/auth/permissionKeys';
import {
  useAppearanceSettings,
  useResetAppearanceOverrides,
  useSaveAppearanceSettings,
} from '../hooks/useAppearanceSettings';
import type { AppearanceSettings } from '../types';
import { AppearanceKnobFields } from '@resources/components/settings/AppearanceKnobFields';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';

/**
 * The app-wide look: what every account renders until it says otherwise, and
 * whether accounts are allowed to say otherwise at all.
 *
 * The card renders for everyone rather than hiding from non-operators. A user
 * who cannot change these still benefits from seeing why their own override
 * is unavailable, and the disabled controls name the permission that would
 * unlock them — which a hidden card cannot do.
 *
 * The matching per-account override lives on Profile → Appearance; this card
 * links to it so an operator can find their own settings from here. Same
 * split, and the same reasons for it, as the toast defaults directly below.
 */
export function AppearanceDefaultsCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: settings } = useAppearanceSettings();
  const saveSettings = useSaveAppearanceSettings();
  const resetOverrides = useResetAppearanceOverrides();
  const { allowed: canManage, reason: permissionReason } = usePermissionGate(PERM.SETTINGS_MANAGE);

  const [draft, setDraft] = useState<AppearanceSettings>(APPEARANCE_FALLBACK);
  const [resetOpen, setResetOpen] = useState(false);

  const [syncedSettings, setSyncedSettings] = useState<typeof settings>(undefined);
  if (settings !== syncedSettings) {
    setSyncedSettings(settings);
    if (settings) setDraft(settings);
  }

  return (
    <SettingsGroup
      icon={IconPalette}
      title={t('appearance.defaultsTitle')}
      description={t('appearance.defaultsDescription')}
      scope={t('general.scopeGlobal')}
      actions={
        <Button
          permission={PERM.SETTINGS_MANAGE}
          onClick={() => saveSettings.mutate(draft)}
          disabled={saveSettings.isPending || !settings}
        >
          {saveSettings.isPending ? t('general.saving') : tc('actions.save')}
        </Button>
      }
    >
      {/* The Save button and the accent buttons carry their own permission
          tooltips, but the selects and the number field are plain controls
          with nowhere to hang one. Saying it once, above the whole set, is
          what keeps a read-only user from having to guess why every field is
          greyed out. */}
      {!canManage && <Alert variant="info" title={t('appearance.readOnlyTitle')} description={permissionReason} />}

      <p className="text-xs text-muted-foreground">
        <Link to="/profile?tab=appearance" className="text-primary hover:underline">
          {t('appearance.personalHintLink')}
        </Link>{' '}
        {t('appearance.personalHint')}
      </p>

      <AppearanceKnobFields
        value={draft}
        onChange={(next) => setDraft({ ...draft, ...next })}
        disabled={!canManage}
        permission={PERM.SETTINGS_MANAGE}
        idPrefix="appearance-default"
        // This card *is* the app-wide layer, so its reset target is the
        // shipped default rather than anything stored.
        baseFontDefault={APPEARANCE_FALLBACK.baseFontPx}
      />

      <SettingsField
        htmlFor="appearance-allow-override"
        label={t('appearance.allowOverrideLabel')}
        description={t('appearance.allowOverrideDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <div className="space-y-4">
          <Switch
            id="appearance-allow-override"
            checked={draft.allowUserOverride}
            onCheckedChange={(allowUserOverride) => setDraft({ ...draft, allowUserOverride })}
            disabled={!canManage}
            aria-label={t('appearance.allowOverrideLabel')}
          />

          {/* Reset is deliberately separate from Save: it deletes other
              people's choices, which is not something that should ride along
              with a change to the default. It also stays available while
              overrides are switched off, because that is exactly when an
              operator wants to clear the ones already stored. */}
          <div className="flex flex-wrap items-center gap-3">
            <Button
              permission={PERM.SETTINGS_MANAGE}
              type="button"
              variant="destructive"
              size="sm"
              onClick={() => setResetOpen(true)}
              disabled={resetOverrides.isPending}
              aria-label={t('appearance.resetAction')}
            >
              <IconRotate className="size-4" />
              {t('appearance.resetAction')}
            </Button>
            <span className="text-xs text-muted-foreground">{t('appearance.resetHint')}</span>
          </div>
        </div>
      </SettingsField>

      <ConfirmDialog
        open={resetOpen}
        onOpenChange={(nextOpen) => {
          if (!nextOpen && !resetOverrides.isPending) setResetOpen(false);
        }}
        title={t('appearance.resetTitle')}
        description={t('appearance.resetDescription')}
        confirmLabel={t('appearance.resetAction')}
        loading={resetOverrides.isPending}
        onConfirm={() => resetOverrides.mutate(undefined, { onSuccess: () => setResetOpen(false) })}
      />
    </SettingsGroup>
  );
}

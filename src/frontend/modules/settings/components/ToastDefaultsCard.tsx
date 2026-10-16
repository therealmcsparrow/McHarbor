// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { IconBellCog, IconRotate } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Switch } from '@resources/components/ui/Switch';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { TOAST_FALLBACK } from '@resources/hooks/useResolvedToastSettings';
import { PERM } from '@core/auth/permissionKeys';
import { useResetToastOverrides, useSaveToastSettings, useToastSettings } from '../hooks/useToastSettings';
import type { ToastSettings } from '../types';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { ToastKnobFields } from './ToastKnobFields';

/**
 * The app-wide toast default: what every account gets until it says
 * otherwise, and whether accounts are allowed to say otherwise at all.
 *
 * The card renders for everyone rather than hiding from non-operators. A
 * user who cannot change these still benefits from seeing why their own
 * override is unavailable, and the disabled controls name the permission
 * that would unlock them — which a hidden card cannot do.
 *
 * The matching per-account override lives on Profile → Notifications; this
 * card links to it so an operator can find their own settings from here.
 */
export function ToastDefaultsCard() {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const { data: settings } = useToastSettings();
  const saveSettings = useSaveToastSettings();
  const resetOverrides = useResetToastOverrides();
  const { allowed: canManage, reason: permissionReason } = usePermissionGate(PERM.SETTINGS_MANAGE);

  const [draft, setDraft] = useState<ToastSettings>(TOAST_FALLBACK);
  const [resetOpen, setResetOpen] = useState(false);

  const [syncedSettings, setSyncedSettings] = useState<typeof settings>(undefined);
  if (settings !== syncedSettings) {
    setSyncedSettings(settings);
    if (settings) setDraft(settings);
  }

  return (
    <SettingsGroup
      icon={IconBellCog}
      title={t('notifications.defaultsTitle')}
      description={t('notifications.defaultsDescription')}
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
      {/* The Save button and the position buttons carry their own permission
          tooltips, but the number inputs and switches are plain controls with
          nowhere to hang one. Saying it once, above the whole set, is what
          keeps a read-only user from having to guess why every field is
          greyed out. */}
      {!canManage && <Alert variant="info" title={t('notifications.readOnlyTitle')} description={permissionReason} />}

      {/* The personal override used to sit directly below this card. It now
          lives on the profile page, so the pointer is what keeps the two
          halves of the setting connected. */}
      <p className="text-xs text-muted-foreground">
        <Link to="/profile?tab=notifications" className="text-primary hover:underline">
          {t('notifications.personalHintLink')}
        </Link>{' '}
        {t('notifications.personalHint')}
      </p>

      <ToastKnobFields
        value={draft}
        onChange={(next) => setDraft({ ...draft, ...next })}
        disabled={!canManage}
        permission={PERM.SETTINGS_MANAGE}
        idPrefix="toast-default"
      />

      <SettingsField
        htmlFor="toast-allow-override"
        label={t('notifications.allowOverrideLabel')}
        description={t('notifications.allowOverrideDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <div className="space-y-4">
          <Switch
            id="toast-allow-override"
            checked={draft.allowUserOverride}
            onCheckedChange={(allowUserOverride) => setDraft({ ...draft, allowUserOverride })}
            disabled={!canManage}
            aria-label={t('notifications.allowOverrideLabel')}
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
              aria-label={t('notifications.resetAction')}
            >
              <IconRotate className="size-4" />
              {t('notifications.resetAction')}
            </Button>
            <span className="text-xs text-muted-foreground">{t('notifications.resetHint')}</span>
          </div>
        </div>
      </SettingsField>

      <ConfirmDialog
        open={resetOpen}
        onOpenChange={(isOpen) => {
          if (!isOpen && !resetOverrides.isPending) setResetOpen(false);
        }}
        title={t('notifications.resetTitle')}
        description={t('notifications.resetDescription')}
        confirmLabel={t('notifications.resetAction')}
        loading={resetOverrides.isPending}
        onConfirm={() => resetOverrides.mutate(undefined, { onSuccess: () => setResetOpen(false) })}
      />
    </SettingsGroup>
  );
}

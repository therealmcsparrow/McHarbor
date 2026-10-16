// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBell } from '@tabler/icons-react';
import { toast } from 'sonner';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { useResolvedToastSettings } from '@resources/hooks/useResolvedToastSettings';
import { useAuth } from '@core/auth/useAuth';
import { useUpdatePreferences } from '@core/auth/usePreferences';
import { SettingsField, SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { ToastKnobFields, type ToastKnobs } from '@resources/components/settings/ToastKnobFields';

type NotificationsCardProps = {
  /** Surface material, so the card can sit on the bare profile canvas. */
  className?: string;
};

/**
 * This account's own toast settings, rendered on Profile → Notifications.
 *
 * The card has two states and the switch at the top decides which. Off, the
 * account follows whatever the operator set and keeps following it when the
 * operator changes it later. On, these five values apply instead.
 *
 * When the operator has disallowed overrides the switch is disabled and the
 * card explains why, rather than disappearing: a user whose toasts sit
 * somewhere they did not choose should be able to find out that the choice
 * was made for them, and by whom.
 *
 * It stays in the settings module because the hooks, the position picker and
 * the shared knob fields it uses all live here alongside the app-wide default
 * it overrides. Only its home is in profile.
 */
export function NotificationsCard({ className }: NotificationsCardProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const updatePreferences = useUpdatePreferences();
  const resolved = useResolvedToastSettings();

  // `overridden` is the server's answer, `useOwn` the form's. They start
  // equal and diverge while the user is deciding.
  const [useOwn, setUseOwn] = useState(resolved.overridden);
  const [draft, setDraft] = useState<ToastKnobs>({
    position: resolved.position,
    durationMs: resolved.durationMs,
    maxVisible: resolved.maxVisible,
    richColors: resolved.richColors,
    closeButton: resolved.closeButton,
  });

  // Re-seed from the resolved values whenever the server's answer changes —
  // a save here, or an operator's reset landing through the session query.
  // Turning the switch on therefore starts from the current default rather
  // than from a blank form, which is what "customize this" should mean.
  const resolvedKey = [
    resolved.overridden,
    resolved.position,
    resolved.durationMs,
    resolved.maxVisible,
    resolved.richColors,
    resolved.closeButton,
  ].join('|');
  const [seededKey, setSeededKey] = useState(resolvedKey);
  if (seededKey !== resolvedKey) {
    setSeededKey(resolvedKey);
    setUseOwn(resolved.overridden);
    setDraft({
      position: resolved.position,
      durationMs: resolved.durationMs,
      maxVisible: resolved.maxVisible,
      richColors: resolved.richColors,
      closeButton: resolved.closeButton,
    });
  }

  async function handleSave() {
    try {
      await updatePreferences.mutateAsync(
        useOwn
          ? {
              toastPosition: draft.position,
              toastDurationMs: draft.durationMs,
              toastMaxVisible: draft.maxVisible,
              toastRichColors: draft.richColors,
              // A toast that never auto-dismisses needs a way out; the
              // fields component locks the switch on for that case, and this
              // mirrors it so a stale draft cannot post the rejected pairing.
              toastCloseButton: draft.durationMs === 0 ? true : draft.closeButton,
            }
          : { clearToastOverride: true },
      );
      toast.success(t('general.preferencesSaved'));
    } catch {
      // The preferences mutation reports its own failure through the global mutation cache.
    }
  }

  if (!user) return null;

  return (
    <SettingsGroup
      icon={IconBell}
      title={t('notifications.title')}
      description={t('notifications.description')}
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
          title={t('notifications.overrideBlockedTitle')}
          description={t('notifications.overrideBlockedBody')}
        />
      )}

      <SettingsField
        htmlFor="toast-use-own"
        label={t('notifications.useOwnLabel')}
        description={t('notifications.useOwnDescription')}
        className="rounded-lg border border-border bg-background/40 p-4"
      >
        <Switch
          id="toast-use-own"
          checked={useOwn}
          onCheckedChange={setUseOwn}
          disabled={!resolved.allowUserOverride}
          aria-label={t('notifications.useOwnLabel')}
        />
      </SettingsField>

      <ToastKnobFields
        value={draft}
        onChange={setDraft}
        disabled={!useOwn || !resolved.allowUserOverride}
        idPrefix="toast-user"
      />
    </SettingsGroup>
  );
}

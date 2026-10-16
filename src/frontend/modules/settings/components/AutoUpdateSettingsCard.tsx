// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// AutoUpdateSettingsCard — whether McHarbor installs its own updates once the
// periodic check finds one. The agent half of this card moved to
// `AgentUpdateSettingsCard` when Updates split into Containers / Agents /
// McHarbor sections.

import { useTranslation } from 'react-i18next';
import { useIsNative } from '@resources/hooks/useInstallKind';
import { IconRocket } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import {
  useSaveSelfUpdateSettings,
  useSelfUpdateSettings,
  useSelfUpdateState,
  type SelfUpdateSettings,
} from '../hooks/useUpdates';
import { GatedToggleRow } from './GatedToggleRow';

export function AutoUpdateSettingsCard() {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const { data: settings, isFetching } = useSelfUpdateSettings();
  const { data: state } = useSelfUpdateState();
  const save = useSaveSelfUpdateSettings();

  if (!settings) {
    return null;
  }

  const current = settings;
  const busy = isFetching || save.isPending;

  function update(patch: Partial<SelfUpdateSettings>) {
    save.mutate({ ...current, ...patch });
  }

  return (
    <div className="rounded-lg border border-border p-5">
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconRocket className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('about.autoUpdate.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('about.autoUpdate.subtitle')}</p>
        </div>
      </div>

      <div className="space-y-4">
        <GatedToggleRow
          label={t('about.autoUpdate.installLabel')}
          hint={t(native ? 'about.autoUpdate.installHintNative' : 'about.autoUpdate.installHint')}
          checked={settings.autoInstall}
          disabled={busy || !settings.enabled}
          permission={PERM.SYSTEM_MANAGE}
          onChange={(v) => update({ autoInstall: v })}
        />
        {settings.autoInstall && state?.autoInstallError && (
          <p className="rounded-md bg-destructive/10 p-3 text-xs text-destructive">
            {t('about.autoUpdate.installFailed', { version: state.autoInstallVersion })}
          </p>
        )}
        {!settings.enabled && <p className="text-xs text-muted-foreground">{t('about.autoUpdate.checkerDisabled')}</p>}
      </div>
    </div>
  );
}

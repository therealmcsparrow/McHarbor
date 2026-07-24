// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBell, IconClock, IconShieldCheck } from '@tabler/icons-react';
import { Switch } from '@resources/components/ui/Switch';
import { NumberInput } from '@resources/components/ui/NumberInput';
import {
  useSaveSelfUpdateSettings,
  useSelfUpdateSettings,
} from '../hooks/useUpdates';
import { useNotificationChannels } from '../hooks/useNotificationChannels';

export function SelfUpdateSettingsCard() {
  const { t } = useTranslation('settings');
  const { data: settings, isFetching } = useSelfUpdateSettings();
  const save = useSaveSelfUpdateSettings();

  const { data: allChannels = [] } = useNotificationChannels();
  const channels = allChannels.filter((c) => c.enabled);

  if (!settings) {
    return null;
  }

  const current = settings;

  function update(patch: Partial<typeof current>) {
    save.mutate({ ...current, ...patch });
  }

  function toggleChannel(channelId: string) {
    const next = current.channelIds.includes(channelId)
      ? current.channelIds.filter((id) => id !== channelId)
      : [...current.channelIds, channelId];
    update({ channelIds: next });
  }

  return (
    <div className="rounded-lg border border-border p-5">
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconBell className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">
            {t('about.selfUpdate.title')}
          </h3>
          <p className="text-xs text-muted-foreground">
            {t('about.selfUpdate.subtitle')}
          </p>
        </div>
      </div>

      <div className="space-y-4">
        {/* Enable toggle */}
        <label className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">
              {t('about.selfUpdate.enabledLabel')}
            </p>
            <p className="text-xs text-muted-foreground">
              {t('about.selfUpdate.enabledHint')}
            </p>
          </div>
          <Switch
            checked={settings.enabled}
            onCheckedChange={(v) => update({ enabled: v })}
            disabled={isFetching || save.isPending}
          />
        </label>

        {/* Check interval */}
        <div>
          <label className="mb-1.5 flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <IconClock className="size-3.5" />
            {t('about.selfUpdate.intervalLabel')}
          </label>
          <div className="flex items-center gap-2">
            <NumberInput
              value={settings.intervalHours}
              min={1}
              max={168}
              onChange={(v) => update({ intervalHours: v })}
              disabled={isFetching || save.isPending || !settings.enabled}
              className="w-32"
            />
            <span className="text-xs text-muted-foreground">
              {t('about.selfUpdate.intervalSuffix')}
            </span>
          </div>
        </div>

        {/* Notification channels */}
        <div>
          <label className="mb-1.5 flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <IconShieldCheck className="size-3.5" />
            {t('about.selfUpdate.channelsLabel')}
          </label>
          {channels.length === 0 ? (
            <p className="rounded-md border border-dashed border-border bg-muted/30 p-3 text-xs text-muted-foreground">
              {t('about.selfUpdate.noChannels')}
            </p>
          ) : (
            <div className="space-y-1.5">
              {channels.map((c) => {
                const checked = settings.channelIds.includes(c.id);
                return (
                  <label
                    key={c.id}
                    className="flex items-center gap-3 rounded-md border border-border bg-card px-3 py-2 text-sm"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="font-medium text-foreground truncate">{c.name}</div>
                      <div className="text-xs text-muted-foreground">{c.channelType}</div>
                    </div>
                    <Switch
                      checked={checked}
                      onCheckedChange={() => toggleChannel(c.id)}
                      disabled={isFetching || save.isPending || !settings.enabled}
                      aria-label={c.name}
                    />
                  </label>
                );
              })}
            </div>
          )}
          <p className="mt-1 text-xs text-muted-foreground">
            {t('about.selfUpdate.channelsHint')}
          </p>
        </div>
      </div>
    </div>
  );
}
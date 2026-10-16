// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { AlertDestinationInput } from '@resources/hooks/useAlerts';
import type { CommunicationChannel } from '../hooks/useNotificationChannels';
import { ALERT_SEVERITIES, ALERT_SEVERITY_LABEL_KEYS } from './alert-options';

type AlertDestinationsFieldProps = {
  channels: CommunicationChannel[];
  destinations: AlertDestinationInput[];
  sendInApp: boolean;
  onDestinationsChange: (destinations: AlertDestinationInput[]) => void;
  onSendInAppChange: (sendInApp: boolean) => void;
  onOpenCommunications: () => void;
};

/**
 * Where a rule delivers.
 *
 * A rule can name any number of channels — built-in or plugin-owned,
 * they are the same rows — and each entry carries an optional minimum
 * severity, so one rule can page a messenger only on critical while
 * always posting to a team chat channel.
 */
export function AlertDestinationsField({
  channels,
  destinations,
  sendInApp,
  onDestinationsChange,
  onSendInAppChange,
  onOpenCommunications,
}: AlertDestinationsFieldProps) {
  const { t } = useTranslation('settings');

  const selected = new Map(destinations.map((destination) => [destination.channelId, destination]));

  // A destination whose channel was deleted still has to be visible —
  // otherwise saving the rule would silently drop it.
  const orphaned = destinations.filter(
    (destination) => !channels.some((channel) => channel.id === destination.channelId),
  );

  function toggleChannel(channelId: string, checked: boolean) {
    if (checked) {
      onDestinationsChange([...destinations, { channelId }]);
      return;
    }
    onDestinationsChange(destinations.filter((destination) => destination.channelId !== channelId));
  }

  function setMinSeverity(channelId: string, minSeverity: string) {
    onDestinationsChange(
      destinations.map((destination) =>
        destination.channelId === channelId ? { ...destination, minSeverity } : destination,
      ),
    );
  }

  const severityOptions = [
    { value: '', label: t('alerts.minSeverityAny') },
    ...ALERT_SEVERITIES.map((severity) => ({
      value: severity,
      label: t(ALERT_SEVERITY_LABEL_KEYS[severity]),
    })),
  ];

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t('alerts.destinationsLabel')}</Label>
        <p className="text-xs text-muted-foreground">{t('alerts.destinationsHint')}</p>
      </div>

      {channels.length === 0 ? (
        <div className="flex flex-col gap-3 rounded-lg border border-dashed border-border bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-start gap-3">
            <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">{t('alerts.noChannelsInlineHint')}</p>
          </div>
          <Button variant="outline" size="sm" onClick={onOpenCommunications}>
            {t('alerts.openCommunications')}
          </Button>
        </div>
      ) : (
        <div className="divide-y divide-border rounded-lg border border-border">
          {channels.map((channel) => {
            const entry = selected.get(channel.id);
            const checked = Boolean(entry);

            return (
              <div key={channel.id} className="flex flex-wrap items-center gap-3 p-3">
                <Checkbox
                  id={`alert-destination-${channel.id}`}
                  checked={checked}
                  onCheckedChange={(value) => toggleChannel(channel.id, value === true)}
                  aria-label={channel.name}
                />
                <label htmlFor={`alert-destination-${channel.id}`} className="min-w-0 flex-1 cursor-pointer">
                  <span className="block truncate text-sm text-foreground">{channel.name}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {channel.enabled ? channel.channelType : `${channel.channelType} · ${t('communications.disabled')}`}
                  </span>
                </label>

                {checked ? (
                  <div className="w-44">
                    <Select
                      value={entry?.minSeverity ?? ''}
                      onChange={(value) => setMinSeverity(channel.id, value)}
                      options={severityOptions}
                      searchable={false}
                      ariaLabel={t('alerts.minSeverityLabel')}
                    />
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      )}

      {orphaned.length > 0 ? (
        <div className="rounded-lg border border-dashed border-destructive/50 bg-destructive/5 p-3">
          <p className="text-xs text-destructive">{t('alerts.orphanedDestinations', { count: orphaned.length })}</p>
        </div>
      ) : null}

      <div className="flex items-start justify-between gap-4 rounded-lg border border-border bg-muted/20 p-4">
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">{t('alerts.inAppLabel')}</p>
          <p className="text-xs text-muted-foreground">{t('alerts.inAppDescription')}</p>
        </div>
        <Switch aria-label={t('alerts.inAppLabel')} checked={sendInApp} onCheckedChange={onSendInAppChange} />
      </div>
    </div>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { IconAlertTriangle, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAlerts, type AlertRule } from '../hooks/useAlerts';
import { useNotificationChannels } from '../hooks/useNotificationChannels';
import { AlertRuleCard } from './AlertRuleCard';
import { AlertRuleDialog } from './AlertRuleDialog';

export function AlertsTab() {
  const { t } = useTranslation('settings');
  const [, setSearchParams] = useSearchParams();
  const { data: alertsPage, isLoading: alertsLoading } = useAlerts();
  const { data: channels, isLoading: channelsLoading } = useNotificationChannels();
  const [createOpen, setCreateOpen] = useState(false);
  const [editAlert, setEditAlert] = useState<AlertRule | null>(null);

  if (alertsLoading || channelsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  const alertRules = alertsPage?.items ?? [];
  const notificationChannels = channels ?? [];
  const hasChannels = notificationChannels.length > 0;
  const channelNames = new Map(notificationChannels.map((channel) => [channel.id, channel.name]));

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <p className="max-w-3xl text-sm text-muted-foreground">{t('alerts.description')}</p>
        <Button onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('alerts.addRule')}
        </Button>
      </div>

      {!hasChannels ? (
        <div className="flex flex-col gap-4 rounded-lg border border-dashed border-border bg-muted/20 p-4 md:flex-row md:items-center md:justify-between">
          <div className="flex items-start gap-3">
            <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
            <div className="space-y-1">
              <p className="text-sm font-medium text-foreground">{t('alerts.noChannelsTitle')}</p>
              <p className="text-sm text-muted-foreground">{t('alerts.noChannelsDescription')}</p>
            </div>
          </div>
          <Button variant="outline" onClick={() => setSearchParams({ tab: 'communications' })}>
            {t('alerts.openCommunications')}
          </Button>
        </div>
      ) : null}

      {alertRules.length > 0 ? (
        <div className="space-y-3">
          {alertRules.map((alertRule) => (
            <AlertRuleCard
              key={alertRule.id}
              rule={alertRule}
              channelName={alertRule.channelId ? channelNames.get(alertRule.channelId) : undefined}
              onEdit={setEditAlert}
            />
          ))}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
          <IconAlertTriangle className="size-10 text-muted-foreground" />
          <p className="mt-3 text-sm font-medium text-foreground">{t('alerts.noRules')}</p>
          <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('alerts.noRulesDescription')}</p>
        </div>
      )}

      <AlertRuleDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        channels={notificationChannels}
      />

      {editAlert ? (
        <AlertRuleDialog
          open={!!editAlert}
          onOpenChange={(open) => {
            if (!open) {
              setEditAlert(null);
            }
          }}
          channels={notificationChannels}
          alert={editAlert}
        />
      ) : null}
    </div>
  );
}

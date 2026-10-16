// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { IconAlertTriangle, IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tabs, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { useAlerts, type AlertRule } from '@resources/hooks/useAlerts';
import { useAlertConditions } from '@resources/hooks/useAlertConditions';
import { useNotificationChannels } from '../hooks/useNotificationChannels';
import { AlertHistoryPanel } from './AlertHistoryPanel';
import { TicketsPanel } from './TicketsPanel';
import { AlertRuleCard } from './AlertRuleCard';
import { AlertRuleDialog } from './AlertRuleDialog';
import { PERM } from '@core/auth/permissionKeys';

export function AlertsTab() {
  const { t } = useTranslation('settings');
  const [, setSearchParams] = useSearchParams();
  const { data: alertsPage, isLoading: alertsLoading } = useAlerts();
  const { data: channels, isLoading: channelsLoading } = useNotificationChannels();
  const { data: catalog, isLoading: catalogLoading } = useAlertConditions();
  const [createOpen, setCreateOpen] = useState(false);
  const [editAlert, setEditAlert] = useState<AlertRule | null>(null);
  const [view, setView] = useState('rules');

  if (alertsLoading || channelsLoading || catalogLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner />
      </div>
    );
  }

  const alertRules = alertsPage?.items ?? [];
  const notificationChannels = channels ?? [];
  const hasChannels = notificationChannels.length > 0;

  function openCommunications() {
    setSearchParams({ tab: 'communications' });
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <p className="max-w-3xl text-sm text-muted-foreground">{t('alerts.description')}</p>
        <Button permission={PERM.ALERTS_MANAGE} onClick={() => setCreateOpen(true)}>
          <IconPlus className="mr-2 size-4" />
          {t('alerts.addRule')}
        </Button>
      </div>

      <Tabs value={view} onValueChange={setView}>
        <TabsList>
          <TabsTrigger value="rules">{t('alerts.tabRules')}</TabsTrigger>
          <TabsTrigger value="history">{t('alerts.tabHistory')}</TabsTrigger>
          <TabsTrigger value="tickets">{t('alerts.tabTickets')}</TabsTrigger>
        </TabsList>
      </Tabs>

      {view === 'tickets' ? (
        <TicketsPanel />
      ) : view === 'history' ? (
        <AlertHistoryPanel />
      ) : (
        <div className="space-y-4">
          {!hasChannels ? (
            <div className="flex flex-col gap-4 rounded-lg border border-dashed border-border bg-muted/20 p-4 md:flex-row md:items-center md:justify-between">
              <div className="flex items-start gap-3">
                <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
                <div className="space-y-1">
                  <p className="text-sm font-medium text-foreground">{t('alerts.noChannelsTitle')}</p>
                  <p className="text-sm text-muted-foreground">{t('alerts.noChannelsDescription')}</p>
                </div>
              </div>
              <Button variant="outline" onClick={openCommunications}>
                {t('alerts.openCommunications')}
              </Button>
            </div>
          ) : null}

          {alertRules.length > 0 ? (
            <div className="space-y-3">
              {alertRules.map((alertRule) => (
                <AlertRuleCard key={alertRule.id} rule={alertRule} catalog={catalog} onEdit={setEditAlert} />
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
              <IconAlertTriangle className="size-10 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium text-foreground">{t('alerts.noRules')}</p>
              <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('alerts.noRulesDescription')}</p>
            </div>
          )}
        </div>
      )}

      <AlertRuleDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        channels={notificationChannels}
        onOpenCommunications={openCommunications}
      />

      {editAlert ? (
        <AlertRuleDialog
          open={!!editAlert}
          onOpenChange={(nextOpen) => {
            if (!nextOpen) {
              setEditAlert(null);
            }
          }}
          channels={notificationChannels}
          alert={editAlert}
          onOpenCommunications={openCommunications}
        />
      ) : null}
    </div>
  );
}

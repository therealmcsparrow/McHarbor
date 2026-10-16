// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Spinner } from '@resources/components/ui/Spinner';
import { useAgentSettings, useSaveAgentSettings } from '../hooks/useAgentSettings';
import { AgentDirectTransferTest } from './AgentDirectTransferTest';
import { PERM } from '@core/auth/permissionKeys';

export function AgentTab() {
  const { t } = useTranslation('settings');
  const { data: agentSettings, isLoading } = useAgentSettings();
  const save = useSaveAgentSettings();

  const [eventMode, setEventMode] = useState('poll');
  const [eventPollInterval, setEventPollInterval] = useState(30);
  const [pingInterval, setPingInterval] = useState(30);
  const [metricsEnabled, setMetricsEnabled] = useState(false);
  const [requestTimeout, setRequestTimeout] = useState(30);
  const [statsPushInterval, setStatsPushInterval] = useState(10);
  const [initialized, setInitialized] = useState(false);

  // Sync state from fetched data
  if (agentSettings && !initialized) {
    setEventMode(agentSettings.eventMode);
    setEventPollInterval(agentSettings.eventPollInterval);
    setPingInterval(agentSettings.pingInterval);
    setMetricsEnabled(agentSettings.metricsEnabled);
    setRequestTimeout(agentSettings.requestTimeout);
    setStatsPushInterval(agentSettings.statsPushInterval);
    setInitialized(true);
  }

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  const handleSave = () => {
    save.mutate({
      eventMode,
      eventPollInterval,
      pingInterval,
      metricsEnabled,
      requestTimeout,
      statsPushInterval,
    });
  };

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">{t('agent.description')}</p>

      <AgentDirectTransferTest />

      {/* Event Collection Mode */}
      <div>
        <Label className="mb-2">{t('agent.eventMode')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('agent.eventModeDescription')}</p>
        <Select
          value={eventMode}
          onChange={setEventMode}
          options={[
            { value: 'poll', label: t('agent.eventModePoll') },
            { value: 'stream', label: t('agent.eventModeStream') },
          ]}
          className="max-w-sm"
        />
      </div>

      {/* Stream mode warning */}
      {eventMode === 'stream' && (
        <div className="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/5 p-4">
          <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-warning" />
          <div className="text-sm text-muted-foreground">
            <p className="font-medium text-foreground">{t('agent.streamWarningTitle')}</p>
            <p>{t('agent.streamWarning')}</p>
          </div>
        </div>
      )}

      {/* Event Poll Interval -- only show in poll mode */}
      {eventMode === 'poll' && (
        <div>
          <Label className="mb-2">{t('agent.eventPollInterval')}</Label>
          <p className="mb-2 text-sm text-muted-foreground">{t('agent.eventPollIntervalDescription')}</p>
          <NumberInput value={eventPollInterval} onChange={setEventPollInterval} min={10} max={300} className="w-40" />
        </div>
      )}

      {/* Ping Interval */}
      <div>
        <Label className="mb-2">{t('agent.pingInterval')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('agent.pingIntervalDescription')}</p>
        <NumberInput value={pingInterval} onChange={setPingInterval} min={10} max={120} className="w-40" />
      </div>

      {/* Metrics Collection */}
      <div className="flex items-center justify-between rounded-lg border border-border p-4">
        <div>
          <p className="font-medium text-foreground">{t('agent.metricsEnabled')}</p>
          <p className="text-sm text-muted-foreground">{t('agent.metricsEnabledDescription')}</p>
        </div>
        <Switch checked={metricsEnabled} onCheckedChange={setMetricsEnabled} />
      </div>

      {metricsEnabled && (
        <div>
          <Label className="mb-2">{t('agent.statsPushInterval')}</Label>
          <p className="mb-2 text-sm text-muted-foreground">{t('agent.statsPushIntervalDescription')}</p>
          <NumberInput value={statsPushInterval} onChange={setStatsPushInterval} min={5} max={300} className="w-40" />
        </div>
      )}

      {/* Request Timeout */}
      <div>
        <Label className="mb-2">{t('agent.requestTimeout')}</Label>
        <p className="mb-2 text-sm text-muted-foreground">{t('agent.requestTimeoutDescription')}</p>
        <NumberInput value={requestTimeout} onChange={setRequestTimeout} min={5} max={120} className="w-40" />
      </div>

      <Button permission={PERM.SETTINGS_MANAGE} onClick={handleSave} disabled={save.isPending}>
        {save.isPending ? t('agent.saving') : t('actions.save', { ns: 'common' })}
      </Button>
    </div>
  );
}

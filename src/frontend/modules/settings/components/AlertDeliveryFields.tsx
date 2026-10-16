// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { useEnvironmentStore } from '@resources/stores/environment';
import { ALERT_COOLDOWN_SECONDS } from './alert-options';
import { AlertTemplateField } from './AlertTemplateField';
import type { AlertConditionConfig } from '@resources/hooks/useAlerts';

type AlertDeliveryFieldsProps = {
  environmentIds: string[];
  cooldownSeconds: number;
  notifyOnRecovery: boolean;
  messageTemplate: string;
  alertName: string;
  severity: string;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  /** Host-wide conditions have no environment, so the scope picker hides. */
  showEnvironmentScope: boolean;
  onEnvironmentIdsChange: (ids: string[]) => void;
  onCooldownChange: (seconds: number) => void;
  onNotifyOnRecoveryChange: (value: boolean) => void;
  onMessageTemplateChange: (value: string) => void;
};

/** Scope, repeat behaviour, recovery, and the message body. */
export function AlertDeliveryFields({
  environmentIds,
  cooldownSeconds,
  notifyOnRecovery,
  messageTemplate,
  alertName,
  severity,
  sourceType,
  conditionKey,
  condition,
  showEnvironmentScope,
  onEnvironmentIdsChange,
  onCooldownChange,
  onNotifyOnRecoveryChange,
  onMessageTemplateChange,
}: AlertDeliveryFieldsProps) {
  const { t } = useTranslation('settings');
  const environments = useEnvironmentStore((state) => state.environments);

  function toggleEnvironment(id: string, checked: boolean) {
    if (checked) {
      onEnvironmentIdsChange([...environmentIds, id]);
      return;
    }
    onEnvironmentIdsChange(environmentIds.filter((value) => value !== id));
  }

  const cooldownOptions = ALERT_COOLDOWN_SECONDS.map((seconds) => ({
    value: String(seconds),
    label: seconds === 0 ? t('alerts.cooldownNone') : formatSeconds(seconds),
  }));

  return (
    <div className="space-y-5">
      {showEnvironmentScope ? (
        <div className="space-y-2">
          <Label>{t('alerts.environmentsLabel')}</Label>
          <p className="text-xs text-muted-foreground">{t('alerts.environmentsHint')}</p>
          <div className="flex flex-wrap gap-3 rounded-lg border border-border p-3">
            {environments.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t('alerts.environmentsEmpty')}</p>
            ) : (
              environments.map((environment) => (
                <label
                  key={environment.id}
                  htmlFor={`alert-environment-${environment.id}`}
                  className="flex cursor-pointer items-center gap-2 text-sm text-foreground"
                >
                  <Checkbox
                    id={`alert-environment-${environment.id}`}
                    checked={environmentIds.includes(environment.id)}
                    onCheckedChange={(value) => toggleEnvironment(environment.id, value === true)}
                    aria-label={environment.name}
                  />
                  {environment.name}
                </label>
              ))
            )}
          </div>
        </div>
      ) : null}

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="space-y-2">
          <Label>{t('alerts.cooldownLabel')}</Label>
          <Select
            value={String(cooldownSeconds)}
            onChange={(value) => onCooldownChange(Number(value))}
            options={cooldownOptions}
            searchable={false}
          />
          <p className="text-xs text-muted-foreground">{t('alerts.cooldownHint')}</p>
        </div>

        <div className="flex items-start justify-between gap-4 rounded-lg border border-border bg-muted/20 p-4">
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">{t('alerts.recoveryLabel')}</p>
            <p className="text-xs text-muted-foreground">{t('alerts.recoveryDescription')}</p>
          </div>
          <Switch
            aria-label={t('alerts.recoveryLabel')}
            checked={notifyOnRecovery}
            onCheckedChange={onNotifyOnRecoveryChange}
          />
        </div>
      </div>

      <AlertTemplateField
        value={messageTemplate}
        onChange={onMessageTemplateChange}
        name={alertName}
        severity={severity}
        sourceType={sourceType}
        conditionKey={conditionKey}
        condition={condition}
      />
    </div>
  );
}

function formatSeconds(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400}d`;
  if (seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

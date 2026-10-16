// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { HealthcheckConfig } from '../../types/edit-form';
import { EditInput, ToggleField } from './EditFieldControls';
import { HealthcheckTestButton } from './HealthcheckTestButton';

type Translator = (key: string) => string;

/**
 * The configured healthcheck, read-only.
 *
 * `EnvironmentHealthcheckSection` below is edit-only by construction - every
 * field is a ToggleField or EditInput - and it was rendered only when
 * `editing` was true. The consequence was that a container's health
 * *configuration* was invisible unless you first unlocked the editor: the
 * Overview card shows probe results, but nothing anywhere showed the command,
 * interval or retries that produced them.
 *
 * Values come from the inspect payload rather than the edit form, because the
 * edit form only exists while editing. Docker reports the durations in
 * nanoseconds, and `Test[0]` is the mode ("CMD-SHELL", or "NONE" when no
 * healthcheck is configured) with the command in the remaining elements.
 */
export function EnvironmentHealthcheckSummary({
  t,
  healthcheck,
}: {
  t: Translator;
  healthcheck?: {
    Test: string[] | null;
    Interval: number;
    Timeout: number;
    Retries: number;
    StartPeriod: number;
  } | null;
}) {
  const test = healthcheck?.Test ?? [];
  const configured = test.length > 0 && test[0] !== 'NONE';
  const seconds = (ns: number) => (ns ? Math.round(ns / 1e9) : 0);

  const rows: Array<[string, string]> = configured
    ? [
        [t('environment.healthCommand'), test.slice(1).join(' ')],
        [t('environment.healthInterval'), String(seconds(healthcheck?.Interval ?? 0))],
        [t('environment.healthTimeout'), String(seconds(healthcheck?.Timeout ?? 0))],
        [t('environment.healthRetries'), String(healthcheck?.Retries ?? 0)],
        [t('environment.healthStartPeriod'), String(seconds(healthcheck?.StartPeriod ?? 0))],
      ]
    : [];

  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.healthcheck')}
      </h3>
      {!configured ? (
        <p className="text-xs text-muted-foreground">{t('environment.healthNotConfigured')}</p>
      ) : (
        <div>
          {rows.map(([label, value]) => (
            <div key={label} className="flex gap-2 border-b border-border py-1.5 last:border-0">
              <span className="shrink-0 font-mono text-xs font-medium text-foreground">{label}</span>
              <span className="truncate font-mono text-xs text-muted-foreground">{value}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

export function EnvironmentHealthcheckSection({
  t,
  containerId,
  running,
  healthcheck,
  onHealthcheckChange,
}: {
  t: Translator;
  containerId: string;
  running: boolean;
  healthcheck: HealthcheckConfig;
  onHealthcheckChange: <K extends keyof HealthcheckConfig>(field: K, value: HealthcheckConfig[K]) => void;
}) {
  return (
    <div className="mc-glass rounded-lg p-6 lg:col-span-2">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('environment.healthcheck')}
      </h3>
      <div className="space-y-3">
        <ToggleField
          label={t('environment.healthEnabled')}
          checked={healthcheck.enabled}
          onChange={(value) => onHealthcheckChange('enabled', value)}
        />
        {healthcheck.enabled && (
          <>
            <EditInput
              label={t('environment.healthCommand')}
              value={healthcheck.command}
              onChange={(value) => onHealthcheckChange('command', value)}
              placeholder={t('placeholders.healthcheckCommand')}
            />
            <HealthcheckTestButton
              containerId={containerId}
              running={running}
              command={healthcheck.command}
              timeoutSeconds={healthcheck.timeout}
            />
            <div className="grid grid-cols-2 gap-3">
              <EditInput
                label={t('environment.healthInterval')}
                numeric
                value={healthcheck.interval}
                onChange={(value) => onHealthcheckChange('interval', parseInt(value, 10) || 30)}
              />
              <EditInput
                label={t('environment.healthTimeout')}
                numeric
                value={healthcheck.timeout}
                onChange={(value) => onHealthcheckChange('timeout', parseInt(value, 10) || 30)}
              />
              <EditInput
                label={t('environment.healthRetries')}
                numeric
                value={healthcheck.retries}
                onChange={(value) => onHealthcheckChange('retries', parseInt(value, 10) || 3)}
              />
              <EditInput
                label={t('environment.healthStartPeriod')}
                numeric
                value={healthcheck.startPeriod}
                onChange={(value) => onHealthcheckChange('startPeriod', parseInt(value, 10) || 0)}
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}

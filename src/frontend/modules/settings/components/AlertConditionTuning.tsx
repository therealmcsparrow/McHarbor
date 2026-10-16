// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { AlertCondition } from '@resources/hooks/useAlertConditions';
import type { AlertConditionConfig } from '@resources/hooks/useAlerts';
import { ALERT_DURATION_SECONDS } from './alert-options';
import { formatSeconds } from './alertRuleFormat';

type AlertConditionTuningProps = {
  activeCondition: AlertCondition | undefined;
  condition: AlertConditionConfig;
  target: string;
  onConditionChange: (condition: AlertConditionConfig) => void;
  onTargetChange: (target: string) => void;
};

/**
 * The operator / threshold / window / duration / target inputs of the
 * rule builder, plus any extra fields the catalog condition declared.
 * Which of them render is decided by the condition's declared kind.
 */
export function AlertConditionTuning({
  activeCondition,
  condition,
  target,
  onConditionChange,
  onTargetChange,
}: AlertConditionTuningProps) {
  const { t } = useTranslation('settings');

  function updateCondition(patch: Partial<AlertConditionConfig>) {
    onConditionChange({ ...condition, ...patch });
  }

  function updateParam(key: string, value: string) {
    onConditionChange({ ...condition, params: { ...condition.params, [key]: value } });
  }

  const durationOptions = ALERT_DURATION_SECONDS.map((seconds) => ({
    value: String(seconds),
    label: seconds === 0 ? t('alerts.durationImmediate') : formatSeconds(seconds),
  }));

  const showThreshold = activeCondition?.kind === 'threshold' || activeCondition?.kind === 'count_in_window';
  const showWindow = activeCondition?.kind === 'count_in_window';
  const showFor = activeCondition?.kind === 'duration' || (activeCondition?.supportsFor ?? false);
  const showTarget = activeCondition ? activeCondition.targetKind !== 'none' : false;

  return (
    <>
      {showThreshold ? (
        <>
          <div className="space-y-2">
            <Label>{t('alerts.operatorLabel')}</Label>
            <Select
              value={condition.operator ?? activeCondition?.defaultOperator ?? '>'}
              onChange={(value) => updateCondition({ operator: value })}
              options={(activeCondition?.operators ?? ['>']).map((operator) => ({
                value: operator,
                label: operator,
              }))}
              searchable={false}
            />
          </div>

          <div className="space-y-2">
            <Label>
              {t('alerts.thresholdLabel')}
              {activeCondition?.unit ? ` (${activeCondition.unit})` : ''}
            </Label>
            <NumberInput
              aria-label={t('alerts.thresholdLabel')}
              value={condition.threshold ?? activeCondition?.defaultThreshold ?? 0}
              onChange={(threshold) => updateCondition({ threshold })}
            />
          </div>
        </>
      ) : null}

      {showWindow ? (
        <div className="space-y-2">
          <Label>{t('alerts.windowLabel')}</Label>
          <Select
            value={String(condition.windowSeconds ?? 900)}
            onChange={(value) => updateCondition({ windowSeconds: Number(value) })}
            options={durationOptions.filter((option) => option.value !== '0')}
            searchable={false}
          />
          <p className="text-xs text-muted-foreground">{t('alerts.windowHint')}</p>
        </div>
      ) : null}

      {showFor ? (
        <div className="space-y-2">
          <Label>{activeCondition?.kind === 'duration' ? t('alerts.forLabelDuration') : t('alerts.forLabel')}</Label>
          <Select
            value={String(condition.forSeconds ?? 0)}
            onChange={(value) => updateCondition({ forSeconds: Number(value) })}
            options={durationOptions}
            searchable={false}
          />
          <p className="text-xs text-muted-foreground">{t('alerts.forHint')}</p>
        </div>
      ) : null}

      {showTarget ? (
        <div className="space-y-2 sm:col-span-2">
          <Label>
            {activeCondition?.targetKind === 'environment'
              ? t('alerts.targetLabelEnvironment')
              : t('alerts.targetLabel')}
          </Label>
          <Input
            value={target}
            onChange={(changeEvent) => onTargetChange(changeEvent.target.value)}
            placeholder={t('alerts.targetPlaceholder')}
          />
          <p className="text-xs text-muted-foreground">{t('alerts.targetHint')}</p>
        </div>
      ) : null}

      {(activeCondition?.fields ?? []).map((field) => (
        <div key={field.key} className="space-y-2">
          <Label>{field.label}</Label>
          <Input
            value={condition.params?.[field.key] ?? ''}
            onChange={(changeEvent) => updateParam(field.key, changeEvent.target.value)}
            placeholder={field.placeholder}
          />
          {field.help ? <p className="text-xs text-muted-foreground">{field.help}</p> : null}
        </div>
      ))}
    </>
  );
}

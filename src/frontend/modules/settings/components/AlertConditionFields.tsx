// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { AlertCondition, AlertConditionCatalog } from '@resources/hooks/useAlertConditions';
import type { AlertConditionConfig } from '@resources/hooks/useAlerts';
import { AlertConditionTuning } from './AlertConditionTuning';

type AlertConditionFieldsProps = {
  catalog: AlertConditionCatalog | undefined;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  target: string;
  onSourceChange: (sourceType: string, conditionKey: string) => void;
  onConditionChange: (condition: AlertConditionConfig) => void;
  onTargetChange: (target: string) => void;
};

/**
 * The condition half of the rule builder.
 *
 * Everything rendered here is driven by the catalog declaration: which
 * operators exist, whether a threshold or a duration applies, what unit
 * to suffix, and any extra fields the condition asked for. A plugin
 * that registers a condition therefore gets a correct form without a
 * line of frontend code.
 */
export function AlertConditionFields({
  catalog,
  sourceType,
  conditionKey,
  condition,
  target,
  onSourceChange,
  onConditionChange,
  onTargetChange,
}: AlertConditionFieldsProps) {
  const { t } = useTranslation('settings');

  const sources = catalog?.sources ?? [];
  const activeSource = sources.find((source) => source.sourceType === sourceType);
  const activeCondition = activeSource?.conditions.find((item) => item.key === conditionKey);

  function conditionLabel(item: AlertCondition): string {
    // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: alert conditions are contributed by sources, which may ship no bundle
    return item.i18nKey ? t(`${item.i18nKey}.label`, { defaultValue: item.displayName }) : item.displayName;
  }

  function conditionHelp(item: AlertCondition): string | undefined {
    if (!item.description) return undefined;
    return item.i18nKey
      ? // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: alert conditions are contributed by sources, which may ship no bundle
        t(`${item.i18nKey}.description`, { defaultValue: item.description })
      : item.description;
  }

  // One flat list, prefixed by source, rather than a grouped select:
  // the Select primitive has no option groups, and a prefix keeps
  // "Containers / CPU usage" distinguishable from "Hosts / CPU usage".
  const conditionOptions = sources.flatMap((source) =>
    source.conditions.map((item) => ({
      value: `${source.sourceType}:${item.key}`,
      label: `${source.displayName} — ${conditionLabel(item)}`,
    })),
  );

  return (
    <div className="grid gap-5 sm:grid-cols-2">
      <div className="space-y-2 sm:col-span-2">
        <Label>{t('alerts.conditionPickerLabel')}</Label>
        <Select
          value={activeCondition ? `${sourceType}:${conditionKey}` : ''}
          onChange={(value) => {
            const [nextSource = '', ...rest] = value.split(':');
            onSourceChange(nextSource, rest.join(':'));
          }}
          options={conditionOptions}
          placeholder={t('alerts.conditionPickerPlaceholder')}
        />
        {activeCondition ? (
          <p className="text-xs text-muted-foreground">{conditionHelp(activeCondition)}</p>
        ) : (
          <p className="text-xs text-muted-foreground">{t('alerts.conditionPickerHint')}</p>
        )}
        {activeSource && !activeSource.builtIn ? (
          <p className="text-xs text-muted-foreground">
            {t('alerts.conditionFromPlugin', { plugin: activeSource.slug })}
          </p>
        ) : null}
      </div>

      <AlertConditionTuning
        activeCondition={activeCondition}
        condition={condition}
        target={target}
        onConditionChange={onConditionChange}
        onTargetChange={onTargetChange}
      />
    </div>
  );
}

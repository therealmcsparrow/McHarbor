// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { displayValue } from '../utils/display-value';
import { MetricConditionsField } from './MetricConditionsField';
import { CronField } from './CronField';
import { CodeField } from './CodeField';
import { ConfigTextField } from './ConfigTextField';
import { ConfigSelectField } from './ConfigSelectField';
import { ConfigFieldLabelled } from './ConfigFieldLabelled';
import { ConfigResourceField } from './ConfigResourceField';
import type { ConfigField } from '../types';

interface ConfigFieldRendererProps {
  field: ConfigField;
  value: unknown;
  onChange: (v: unknown) => void;
  nodeConfig: Record<string, unknown>;
  nodeKey?: string;
}

function parseJsonInput(text: string, emptyValue: unknown): unknown {
  const trimmed = text.trim();
  if (trimmed === '') {
    return emptyValue;
  }
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export function ConfigFieldRenderer({ field, value, onChange, nodeConfig, nodeKey }: ConfigFieldRendererProps) {
  const { t } = useTranslation('common');
  const strVal = value != null ? displayValue(value) : displayValue(field.default);
  const fieldLabel = nodeKey
    ? // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
      t(`nodes:${nodeKey}.config.${field.key}`, { defaultValue: field.label })
    : field.label;
  const envId =
    typeof nodeConfig.environment === 'string' && nodeConfig.environment ? nodeConfig.environment : undefined;
  const containerId =
    typeof nodeConfig.container === 'string' && nodeConfig.container ? nodeConfig.container : undefined;
  const optionLabel = (optValue: string, fallback: string) =>
    nodeKey
      ? t(`nodes:${nodeKey}.options.${field.key}.${optValue}`, {
          // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
          defaultValue: fallback,
        })
      : fallback;

  switch (field.type) {
    case 'text':
    case 'expression':
    case 'textarea':
      return <ConfigTextField field={field} strVal={strVal} onChange={onChange} fieldLabel={fieldLabel} />;

    case 'number':
      return (
        <ConfigFieldLabelled label={fieldLabel} required={field.required}>
          <NumberInput value={Number(strVal) || 0} onChange={(v) => onChange(v)} size="sm" />
        </ConfigFieldLabelled>
      );

    case 'select':
      return (
        <ConfigSelectField
          field={field}
          strVal={strVal}
          onChange={onChange}
          fieldLabel={fieldLabel}
          optionLabel={optionLabel}
        />
      );

    case 'toggle':
      return (
        <div className="flex items-center justify-between">
          <Label className="text-xs">{fieldLabel}</Label>
          <Switch aria-label={fieldLabel} checked={Boolean(value)} onCheckedChange={(checked) => onChange(checked)} />
        </div>
      );

    case 'json':
      return (
        <div>
          <Label className="mb-1.5 text-xs">
            {fieldLabel}
            {field.required && <span className="text-destructive"> *</span>}
          </Label>
          <textarea
            value={typeof value === 'string' ? value : JSON.stringify(value ?? '', null, 2)}
            onChange={(e) => onChange(parseJsonInput(e.target.value, ''))}
            rows={4}
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            placeholder="{}"
          />
        </div>
      );

    case 'metric-conditions':
      return <MetricConditionsField value={value} onChange={onChange} />;

    case 'cron':
      return (
        <CronField
          field={field}
          value={strVal}
          onChange={onChange}
          nodeKey={nodeKey}
          timezone={typeof nodeConfig.timezone === 'string' ? nodeConfig.timezone : null}
        />
      );

    case 'key-value':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}</Label>
          <textarea
            value={typeof value === 'string' ? value : JSON.stringify(value ?? {}, null, 2)}
            onChange={(e) => onChange(parseJsonInput(e.target.value, {}))}
            rows={3}
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            placeholder={t('configField.keyValuePlaceholder', { ns: 'workflows' })}
          />
        </div>
      );

    case 'code': {
      const lang = nodeConfig.language === 'typescript' ? 'typescript' : 'javascript';
      return (
        <CodeField field={field} value={strVal} onChange={(v) => onChange(v)} language={lang} fieldLabel={fieldLabel} />
      );
    }

    default:
      return (
        <ConfigResourceField
          field={field}
          value={value}
          onChange={onChange}
          nodeConfig={nodeConfig}
          nodeKey={nodeKey}
          fieldLabel={fieldLabel}
          strVal={strVal}
          envId={envId}
          containerId={containerId}
        />
      );
  }
}

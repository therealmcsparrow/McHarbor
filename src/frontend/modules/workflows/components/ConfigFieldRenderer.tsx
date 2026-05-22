// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Label } from '@resources/components/ui/Label';
import { MetricConditionsField } from './MetricConditionsField';
import { CronField } from './CronField';
import { CodeField } from './CodeField';
import { EnvironmentSelect } from './EnvironmentSelect';
import { ContainerSelect } from './ContainerSelect';
import { LinkOutputSelect } from './LinkOutputSelect';
import { EmailServerSelect } from './EmailServerSelect';
import { CommunicationChannelSelect } from './CommunicationChannelSelect';
import type { ConfigField } from '../types';
import { ct } from '../canvas-theme';

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

function maskSecretValue(value: string): string {
  if (!value) {
    return '';
  }

  const visibleLength = Math.min(value.length, 32);
  const masked = '•'.repeat(visibleLength);
  return value.length > visibleLength ? `${masked}…` : masked;
}

export function ConfigFieldRenderer({ field, value, onChange, nodeConfig, nodeKey }: ConfigFieldRendererProps) {
  const { t } = useTranslation('common');
  const [showSecret, setShowSecret] = useState(false);
  const strVal = value != null ? String(value) : (field.default != null ? String(field.default) : '');
  const fieldLabel = nodeKey ? t(`nodes:${nodeKey}.config.${field.key}`, { defaultValue: field.label }) : field.label;
  const secretToggleLabel = showSecret
    ? t('workflows.hideSensitiveField', { defaultValue: `Hide ${fieldLabel}` })
    : t('workflows.showSensitiveField', { defaultValue: `Show ${fieldLabel}` });

  switch (field.type) {
    case 'text':
    case 'expression':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <div className="flex items-center gap-2">
            <Input
              type={field.secret && !showSecret ? 'password' : 'text'}
              value={strVal}
              onChange={(e) => onChange(e.target.value)}
              className={cn('h-8 text-xs', field.type === 'expression' && 'font-mono')}
            />
            {field.secret ? (
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label={secretToggleLabel}
                onClick={() => setShowSecret((current) => !current)}
              >
                {showSecret ? <IconEyeOff className="size-3.5" /> : <IconEye className="size-3.5" />}
              </Button>
            ) : null}
          </div>
        </div>
      );

    case 'textarea':
      return (
        <div>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <Label className="text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
            {field.secret ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={secretToggleLabel}
                onClick={() => setShowSecret((current) => !current)}
              >
                {showSecret ? t('workflows.hide', { defaultValue: 'Hide' }) : t('workflows.show', { defaultValue: 'Show' })}
              </Button>
            ) : null}
          </div>
          <textarea
            value={field.secret && !showSecret ? maskSecretValue(strVal) : strVal}
            onChange={(e) => onChange(e.target.value)}
            rows={3}
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            readOnly={field.secret && !showSecret}
          />
        </div>
      );

    case 'number':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <NumberInput
            value={Number(strVal) || 0}
            onChange={(v) => onChange(v)}
            size="sm"
          />
        </div>
      );

    case 'select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <select
            value={strVal}
            onChange={(e) => onChange(e.target.value)}
            className="h-8 w-full rounded-md border border-input bg-card px-2 text-xs ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <option value="" className="bg-card text-muted-foreground">{t('workflows.selectPlaceholder')}</option>
            {field.options?.map((opt) => (
              <option key={opt.value} value={opt.value} className="bg-card text-foreground">{nodeKey ? t(`nodes:${nodeKey}.options.${field.key}.${opt.value}`, { defaultValue: opt.label }) : opt.label}</option>
            ))}
          </select>
        </div>
      );

    case 'environment-select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <EnvironmentSelect value={strVal} onChange={(v) => onChange(v)} />
        </div>
      );

    case 'container-select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <ContainerSelect value={strVal} onChange={(v) => onChange(v)} envId={typeof nodeConfig.environment === 'string' ? nodeConfig.environment : undefined} />
        </div>
      );

    case 'email-server-select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <EmailServerSelect value={strVal} onChange={(v) => onChange(v)} />
        </div>
      );

    case 'communication-channel-select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <CommunicationChannelSelect value={strVal} onChange={(v) => onChange(v)} />
        </div>
      );

    case 'toggle':
      return (
        <div className="flex items-center justify-between">
          <Label className="text-xs">{fieldLabel}</Label>
          {/* Raw <button> kept: custom toggle switch with sliding thumb doesn't fit Button's API */}
          <button
            aria-label={fieldLabel}
            onClick={() => onChange(!value)}
            className={cn(
              'relative h-5 w-9 rounded-full transition-colors',
              value ? 'bg-primary' : 'bg-muted',
            )}
          >
            <span className={cn(
              `absolute left-0.5 top-0.5 size-4 rounded-full ${ct.toggleKnob} transition-transform`,
              value ? 'translate-x-4' : '',
            )} />
          </button>
        </div>
      );

    case 'json':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <textarea
            value={typeof value === 'string' ? value : JSON.stringify(value ?? '', null, 2)}
            onChange={(e) => onChange(parseJsonInput(e.target.value, ''))}
            rows={4}
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
            className="w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder='{"key": "value"}'
          />
        </div>
      );

    case 'link-output-select':
      return (
        <div>
          <Label className="mb-1.5 text-xs">{fieldLabel}{field.required && <span className="text-destructive"> *</span>}</Label>
          <LinkOutputSelect value={strVal} onChange={(v) => onChange(v)} />
        </div>
      );

    case 'code': {
      const lang = nodeConfig.language === 'typescript' ? 'typescript' : 'javascript';
      return (
        <CodeField
          field={field}
          value={strVal}
          onChange={(v) => onChange(v)}
          language={lang}
          fieldLabel={fieldLabel}
        />
      );
    }

    default:
      return null;
  }
}


// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import type { AlertConditionConfig } from '@resources/hooks/useAlerts';
import { useAlertTemplatePreview, useAlertTemplateReference } from '../hooks/useAlertTemplate';
import { AlertTemplateReference } from './AlertTemplateReference';

const PREVIEW_STATES = ['firing', 'resolved'] as const;

type Props = {
  value: string;
  onChange: (value: string) => void;
  name: string;
  severity: string;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
};

export function AlertTemplateField({
  value,
  onChange,
  name: ruleName,
  severity,
  sourceType,
  conditionKey,
  condition,
}: Props) {
  const { t } = useTranslation('settings');
  const ref = useRef<HTMLTextAreaElement>(null);
  const [state, setState] = useState<'firing' | 'resolved'>('firing');
  const reference = useAlertTemplateReference(sourceType, conditionKey);
  const preview = useAlertTemplatePreview({
    template: value,
    name: ruleName,
    severity,
    sourceType,
    conditionKey,
    condition,
    state,
  });

  const insert = (text: string) => {
    const el = ref.current;
    const start = el?.selectionStart ?? value.length;
    const end = el?.selectionEnd ?? value.length;
    onChange(value.slice(0, start) + text + value.slice(end));
    requestAnimationFrame(() => {
      el?.focus();
      el?.setSelectionRange(start + text.length, start + text.length);
    });
  };

  const unknown = preview.data?.unknown ?? [];

  return (
    <div className="space-y-2">
      <Label htmlFor="alert-template">{t('alerts.templateLabel')}</Label>
      <p className="text-xs text-muted-foreground">{t('alerts.templateRef.intro')}</p>
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        <Textarea
          id="alert-template"
          ref={ref}
          value={value}
          onChange={(changeEvent) => onChange(changeEvent.target.value)}
          placeholder={t('alerts.templatePlaceholder')}
          className="min-h-[224px] font-mono text-xs"
        />
        <AlertTemplateReference reference={reference.data} onInsert={insert} onReplace={onChange} />
      </div>
      <div className="space-y-2 rounded-md border border-border bg-muted/20 p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs font-medium text-foreground">{t('alerts.templateRef.preview')}</span>
          <fieldset className="m-0 flex min-w-0 gap-1 border-0 p-0" aria-label={t('alerts.templateRef.previewState')}>
            {PREVIEW_STATES.map((s) => (
              <Button
                key={s}
                size="sm"
                variant={state === s ? 'secondary' : 'ghost'}
                aria-pressed={state === s}
                onClick={() => setState(s)}
              >
                {t(`alerts.templateRef.state.${s}`)}
              </Button>
            ))}
          </fieldset>
        </div>
        <p className="text-sm break-words whitespace-pre-wrap text-foreground" data-testid="alert-template-preview">
          {preview.data?.message ?? ''}
        </p>
        {!value.trim() && <p className="text-xs text-muted-foreground">{t('alerts.templateRef.defaultNote')}</p>}
        {unknown.length > 0 && (
          <p className="text-xs text-destructive">{t('alerts.templateRef.unknown', { names: unknown.join(', ') })}</p>
        )}
      </div>
    </div>
  );
}

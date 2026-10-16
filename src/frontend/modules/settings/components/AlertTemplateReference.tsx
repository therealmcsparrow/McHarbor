// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import type { AlertTemplateReference as Reference } from '../hooks/useAlertTemplate';

type Tab = 'variables' | 'context' | 'examples';
const TABS: Tab[] = ['variables', 'context', 'examples'];

const EXAMPLES: { id: string; template: string }[] = [
  { id: 'short', template: '[{{severity}}] {{subject}}: {{value}} ({{rule}})' },
  {
    id: 'placeTime',
    template:
      '{{alert}} is {{state}} on {{subject}} in {{environment}} at {{time}} UTC — {{value}}, limit {{threshold}}.',
  },
  {
    id: 'recovery',
    template: '{{alert}} {{state}}: {{subject}} in {{environment}} was at {{value}}. Fired for {{duration}}.',
  },
  { id: 'ticket', template: '{{alert}} | {{environment}}/{{subject}} | {{rule}} | observed {{value}} | {{timestamp}}' },
];

type Props = {
  reference: Reference | undefined;
  onInsert: (text: string) => void;
  onReplace: (text: string) => void;
};

export function AlertTemplateReference({ reference, onInsert, onReplace }: Props) {
  const { t } = useTranslation('settings');
  const [tab, setTab] = useState<Tab>('variables');
  const context = reference?.context ?? [];

  return (
    <div className="rounded-md border border-border bg-muted/20">
      <div
        className="flex items-center gap-1 border-b border-border p-1"
        role="tablist"
        aria-label={t('alerts.templateRef.title')}
      >
        {TABS.map((id) => (
          <Button
            key={id}
            variant={tab === id ? 'secondary' : 'ghost'}
            size="sm"
            role="tab"
            aria-selected={tab === id}
            onClick={() => setTab(id)}
          >
            {t(`alerts.templateRef.tabs.${id}`)}
            {id === 'context' && context.length > 0 && ` (${context.length})`}
          </Button>
        ))}
      </div>
      <div className="max-h-56 overflow-y-auto p-2 text-xs">
        {tab === 'variables' &&
          (reference?.variables ?? []).map((v) => (
            <Item
              key={v.name}
              code={`{{${v.name}}}`}
              text={t(`alerts.templateRef.vars.${v.name}`)}
              hint={v.sample}
              onClick={() => onInsert(`{{${v.name}}}`)}
            />
          ))}
        {tab === 'context' &&
          (context.length === 0 ? (
            <p className="px-2 py-1 text-muted-foreground">{t('alerts.templateRef.noContext')}</p>
          ) : (
            context.map((c) => (
              <Item
                key={c.key}
                code={`{{ctx.${c.key}}}`}
                text={c.learned ? t('alerts.templateRef.learned') : t(`alerts.templateRef.ctx.${c.key}`)}
                onClick={() => onInsert(`{{ctx.${c.key}}}`)}
              />
            ))
          ))}
        {tab === 'examples' &&
          EXAMPLES.map((e) => (
            <Item
              key={e.id}
              code={e.template}
              text={t(`alerts.templateRef.examples.${e.id}`)}
              onClick={() => onReplace(e.template)}
            />
          ))}
      </div>
    </div>
  );
}

function Item({ code, text, hint, onClick }: { code: string; text: string; hint?: string; onClick: () => void }) {
  return (
    <Button
      variant="ghost"
      className="h-auto w-full flex-col items-start gap-0.5 px-2 py-1 text-left font-normal"
      onClick={onClick}
    >
      <span className="font-mono text-xs text-foreground">{code}</span>
      <span className="text-xs text-muted-foreground">{text}</span>
      {hint && <span className="font-mono text-[10px] text-muted-foreground">{hint}</span>}
    </Button>
  );
}

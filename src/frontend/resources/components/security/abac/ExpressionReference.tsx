// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';
import type { PolicyReference, ReferenceAttribute, ReferenceResourceType } from '@resources/hooks/usePolicyReference';

type Tab = 'variables' | 'helpers' | 'examples';

const TABS: Tab[] = ['variables', 'helpers', 'examples'];

const EXAMPLES = [
  'prodOnly',
  'officeHours',
  'internalNetwork',
  'noApiKeyWrites',
  'teamOwnership',
  'ownResources',
  'largeVms',
  'namePattern',
] as const;

type Props = {
  reference: PolicyReference | undefined;
  resourceType: ReferenceResourceType | undefined;
  onInsert: (text: string) => void;
  onReplace: (text: string) => void;
};

export function ExpressionReference({ reference, resourceType, onInsert, onReplace }: Props) {
  const { t } = useTranslation('abac');
  const [tab, setTab] = useState<Tab>('variables');
  const describe = (a: ReferenceAttribute, fromPlugin: boolean) =>
    fromPlugin ? a.description : t(`reference.vars.${a.path.replace(/\./g, '_')}`);

  return (
    <div className="rounded-md border border-border bg-muted/20">
      <div
        className="flex items-center gap-1 border-b border-border p-1"
        role="tablist"
        aria-label={t('reference.title')}
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
            {t(`reference.tabs.${id}`)}
          </Button>
        ))}
      </div>
      <div className="max-h-56 overflow-y-auto p-2 text-xs">
        {tab === 'variables' && (
          <div className="space-y-3">
            {resourceType && (
              <Section title={t('reference.forType')}>
                {!resourceType.resolvable && (
                  <TypographyText size="xs" tone="muted">
                    {t('editor.noResolver')}
                  </TypographyText>
                )}
                {resourceType.attributes.map((a) => (
                  <Item
                    key={a.path}
                    code={a.path}
                    text={describe(a, resourceType.source !== 'host')}
                    onClick={() => onInsert(a.path)}
                  />
                ))}
              </Section>
            )}
            <Section title={t('reference.everywhere')}>
              {(reference?.variables ?? []).map((a) => (
                <Item key={a.path} code={a.path} text={describe(a, false)} onClick={() => onInsert(a.path)} />
              ))}
            </Section>
          </div>
        )}
        {tab === 'helpers' &&
          (reference?.helpers ?? []).map((h) => (
            <Item
              key={h.name}
              code={h.signature}
              text={t(`reference.helpers.${h.name}`)}
              hint={h.example}
              onClick={() => onInsert(h.example)}
            />
          ))}
        {tab === 'examples' &&
          EXAMPLES.map((id) => (
            <Item
              key={id}
              code={t(`reference.examples.${id}.expression`)}
              text={t(`reference.examples.${id}.title`)}
              onClick={() => onReplace(t(`reference.examples.${id}.expression`))}
            />
          ))}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-1">
      <div className="text-[10px] font-semibold tracking-wide text-muted-foreground uppercase">{title}</div>
      {children}
    </div>
  );
}

function Item({ code, text, hint, onClick }: { code: string; text: string; hint?: string; onClick: () => void }) {
  return (
    <Button
      variant="ghost"
      className={cn('h-auto w-full flex-col items-start gap-0.5 px-2 py-1 text-left font-normal')}
      onClick={onClick}
    >
      <span className="font-mono text-xs text-foreground">{code}</span>
      <span className="text-xs text-muted-foreground">{text}</span>
      {hint && <span className="font-mono text-[10px] text-muted-foreground">{hint}</span>}
    </Button>
  );
}

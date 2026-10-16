// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { ConfigField, NodeDefinition } from '../types';
import { describeFieldType, formatDefaultValue, type MarkdownSection } from './help-tab-utils';

export function FieldCard({ definition, field }: { definition: NodeDefinition; field: ConfigField }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  // i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback
  const translatedLabel = tn(`${definition.key}.config.${field.key}`, { defaultValue: field.label });

  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="text-xs font-medium text-foreground">{translatedLabel}</span>
        <Badge variant="secondary" className="text-[10px]">
          {field.type}
        </Badge>
        <span className="text-[10px] text-muted-foreground">
          {field.required ? t('workflows.required') : t('labels.optional')}
        </span>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{describeFieldType(field.type, t)}</p>
      {field.default != null && (
        <p className="mt-1 text-[10px] text-muted-foreground">
          {t('workflows.defaultValue')}:{' '}
          <code className="rounded bg-muted px-1 py-0.5 text-[10px]">{formatDefaultValue(field.default)}</code>
        </p>
      )}
      {field.options && field.options.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1">
          {field.options.map((option) => (
            <code key={option.value} className="rounded bg-muted px-1 py-0.5 text-[10px]">
              {/* i18n-open-set: workflow node definitions include custom and plugin nodes whose i18n may be absent, so the definition label is the fallback */}
              {tn(`${definition.key}.options.${field.key}.${option.value}`, { defaultValue: option.label })}
            </code>
          ))}
        </div>
      )}
    </div>
  );
}

export function PortCard({ label, description }: { label: string; description: string }) {
  return (
    <div className="rounded-md border border-border p-2.5">
      <div className="flex items-center gap-2">
        <Badge variant="secondary" className="text-[10px]">
          {label}
        </Badge>
      </div>
      <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">{description}</p>
    </div>
  );
}

export function HelpSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="mb-2 text-xs font-semibold text-foreground">{title}</h3>
      <div className="space-y-2">{children}</div>
    </section>
  );
}

export function DocSection({ section }: { section: MarkdownSection }) {
  return (
    <HelpSection title={section.title}>
      <DocSectionContent section={section} />
    </HelpSection>
  );
}

export function DocSectionContent({ section }: { section: MarkdownSection }) {
  return (
    <>
      {section.paragraphs.map((paragraph) => (
        <p key={paragraph} className="text-[11px] leading-relaxed text-muted-foreground">
          {paragraph}
        </p>
      ))}
      {section.bullets.length > 0 && (
        <ul className="space-y-1">
          {section.bullets.map((bullet) => (
            <li key={bullet} className="text-[11px] leading-relaxed text-muted-foreground">
              - {bullet}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

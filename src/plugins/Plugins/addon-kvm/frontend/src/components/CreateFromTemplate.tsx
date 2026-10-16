// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { formatVmMemory } from './vm-resources';
import type { VM } from '../types';

interface TemplateSourceFieldsProps {
  templates: VM[];
  templateId: string;
  linked: boolean;
  onTemplateChange: (id: string) => void;
  onLinkedChange: (linked: boolean) => void;
}

export function TemplateSourceFields({
  templates,
  templateId,
  linked,
  onTemplateChange,
  onLinkedChange,
}: TemplateSourceFieldsProps) {
  const { t } = useTranslation('vm');

  if (templates.length === 0) {
    return <p className="text-sm text-warning">{t('create.source.noTemplates')}</p>;
  }

  return (
    <div className="space-y-4">
      <div>
        <Label>{t('create.source.templateLabel')}</Label>
        <Select
          value={templateId}
          ariaLabel={t('create.source.templateLabel')}
          options={templates.map((entry) => ({
            value: entry.id,
            label: `${entry.name} — ${t('resources.cpu', { count: entry.vcpus })}, ${t('resources.ram', { size: formatVmMemory(entry.memoryMb) })}`,
          }))}
          onChange={onTemplateChange}
        />
      </div>

      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="vm-template-linked">{t('clone.linked')}</Label>
          <p className="text-xs text-muted-foreground">{linked ? t('clone.linkedHint') : t('clone.fullHint')}</p>
        </div>
        <Switch id="vm-template-linked" checked={linked} onCheckedChange={onLinkedChange} />
      </div>

      <p className="text-xs text-muted-foreground">{t('clone.identityNote')}</p>
    </div>
  );
}

export function TemplateSummary({
  name: nameValue,
  template,
  linked,
}: {
  name: string;
  template?: VM;
  linked: boolean;
}) {
  const { t } = useTranslation('vm');

  const facts: [string, string][] = [
    [t('list.columns.name'), nameValue],
    [t('create.source.templateLabel'), template?.name ?? '—'],
    [
      t('create.steps.resources'),
      template
        ? `${t('resources.cpu', { count: template.vcpus })}, ${t('resources.ram', { size: formatVmMemory(template.memoryMb) })}`
        : '—',
    ],
    [t('clone.linked'), linked ? t('clone.linkedHint') : t('clone.fullHint')],
  ];

  return (
    <dl className="grid gap-3 sm:grid-cols-2">
      {facts.map(([label, value]) => (
        <div key={label}>
          <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
          <dd className="mt-0.5 text-sm text-foreground">{value}</dd>
        </div>
      ))}
    </dl>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowUp, IconPlus, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import {
  newSourceRow,
  type AgentReleaseSource,
  type AgentReleaseSourceKind,
  type AgentReleaseSourceRow,
} from '../hooks/useAgentReleases';

type AgentReleaseSourcesFieldProps = {
  sources: AgentReleaseSourceRow[];
  onChange: (sources: AgentReleaseSourceRow[]) => void;
};

export function AgentReleaseSourcesField({ sources, onChange }: AgentReleaseSourcesFieldProps) {
  const { t } = useTranslation('settings');
  const kinds = [
    { value: 'github', label: t('agentReleases.settings.sourceKind.github') },
    { value: 'mirror', label: t('agentReleases.settings.sourceKind.mirror') },
  ];

  function update(index: number, patch: Partial<AgentReleaseSource>) {
    onChange(sources.map((s, i) => (i === index ? { ...s, ...patch } : s)));
  }

  function moveUp(index: number) {
    if (index < 1) return;
    onChange([
      ...sources.slice(0, index - 1),
      ...sources.slice(index, index + 1),
      ...sources.slice(index - 1, index),
      ...sources.slice(index + 1),
    ]);
  }

  return (
    <div className="space-y-2">
      <Label>{t('agentReleases.settings.sources')}</Label>
      <p className="text-xs text-muted-foreground">{t('agentReleases.settings.sourcesHint')}</p>
      <ol className="space-y-2">
        {sources.map((source, index) => (
          <li key={source.id} className="grid gap-2 sm:grid-cols-[12rem_1fr_auto]">
            <Select
              value={source.kind}
              onChange={(kind) => update(index, { kind: kind as AgentReleaseSourceKind })}
              options={kinds}
              ariaLabel={t('agentReleases.settings.sourceKindLabel')}
            />
            <Input
              value={source.url}
              placeholder={t('agentReleases.settings.sourceUrlPlaceholder')}
              aria-label={t('agentReleases.settings.sourceUrl')}
              onChange={(e) => update(index, { url: e.target.value })}
            />
            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                permission={PERM.UPDATES_MANAGE}
                disabled={index === 0}
                aria-label={t('agentReleases.settings.moveUp', {
                  url: source.url,
                })}
                onClick={() => moveUp(index)}
              >
                <IconArrowUp className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                permission={PERM.UPDATES_MANAGE}
                disabled={sources.length === 1}
                aria-label={t('agentReleases.settings.removeSource', {
                  url: source.url,
                })}
                onClick={() => onChange(sources.filter((_, i) => i !== index))}
              >
                <IconX className="size-4" />
              </Button>
            </div>
          </li>
        ))}
      </ol>
      <Button
        variant="outline"
        size="sm"
        permission={PERM.UPDATES_MANAGE}
        disabled={sources.length >= 8}
        onClick={() => onChange([...sources, newSourceRow({ kind: 'mirror', url: '' })])}
      >
        <IconPlus className="mr-1.5 size-4" aria-hidden="true" />
        {t('agentReleases.settings.addSource')}
      </Button>
    </div>
  );
}

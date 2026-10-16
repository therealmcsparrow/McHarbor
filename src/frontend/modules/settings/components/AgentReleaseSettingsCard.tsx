// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCloudDownload } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import {
  newSourceRow,
  useAgentReleases,
  useSaveAgentReleaseSettings,
  type AgentReleaseMode,
  type AgentReleaseSettings,
  type AgentReleaseSourceRow,
} from '../hooks/useAgentReleases';
import { AgentReleaseSourcesField } from './AgentReleaseSourcesField';
import { GatedToggleRow } from './GatedToggleRow';

const PLATFORMS = ['linux/amd64', 'linux/arm64'];

type Draft = Omit<AgentReleaseSettings, 'sources'> & {
  sources: AgentReleaseSourceRow[];
};

function toDraft(s: AgentReleaseSettings): Draft {
  return { ...s, sources: s.sources.map(newSourceRow) };
}

function fromDraft(d: Draft): AgentReleaseSettings {
  return {
    ...d,
    sources: d.sources.map(({ kind, url }) => ({ kind, url: url.trim() })),
  };
}

function ReleaseSettingsForm({ settings, nextCheckAt }: { settings: AgentReleaseSettings; nextCheckAt?: string }) {
  const { t } = useTranslation('settings');
  const save = useSaveAgentReleaseSettings();
  const [draft, setDraft] = useState<Draft>(() => toDraft(settings));
  const dirty = JSON.stringify(fromDraft(draft)) !== JSON.stringify(settings);
  const modes: { value: AgentReleaseMode; label: string }[] = [
    { value: 'notify', label: t('agentReleases.settings.modes.notify') },
    { value: 'download', label: t('agentReleases.settings.modes.download') },
    { value: 'rollout', label: t('agentReleases.settings.modes.rollout') },
  ];
  const patch = (p: Partial<Draft>) => setDraft((d) => ({ ...d, ...p }));
  const togglePlatform = (p: string, on: boolean) =>
    patch({
      imagePlatforms: on ? [...draft.imagePlatforms, p] : draft.imagePlatforms.filter((x) => x !== p),
    });

  return (
    <div className="space-y-5">
      <GatedToggleRow
        label={t('agentReleases.settings.enabled')}
        hint={t('agentReleases.settings.enabledHint')}
        checked={draft.enabled}
        disabled={save.isPending}
        permission={PERM.UPDATES_MANAGE}
        onChange={(enabled) => patch({ enabled })}
      />
      <div className="space-y-1">
        <ReadableScheduleField
          value={draft.checkCron}
          label={t('agentReleases.settings.schedule')}
          onChange={(checkCron) => patch({ checkCron })}
        />
        {nextCheckAt && settings.enabled ? (
          <p className="text-xs text-muted-foreground">
            {t('agentReleases.nextCheck', { date: formatDate(nextCheckAt) })}
          </p>
        ) : null}
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_10rem]">
        <div className="flex flex-col gap-1.5">
          <Label>{t('agentReleases.settings.mode')}</Label>
          <Select
            value={draft.mode}
            onChange={(mode) => patch({ mode: mode as AgentReleaseMode })}
            options={modes}
            ariaLabel={t('agentReleases.settings.mode')}
          />
          <p className="text-xs text-muted-foreground">{t('agentReleases.settings.modeHint')}</p>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="agent-release-keep">{t('agentReleases.settings.keep')}</Label>
          <NumberInput
            id="agent-release-keep"
            min={1}
            max={20}
            value={draft.keep}
            onChange={(keep) => patch({ keep })}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{t('agentReleases.settings.keepHint')}</p>
      <AgentReleaseSourcesField sources={draft.sources} onChange={(sources) => patch({ sources })} />
      <div className="space-y-2">
        <Label>{t('agentReleases.settings.imagePlatforms')}</Label>
        <p className="text-xs text-muted-foreground">{t('agentReleases.settings.imagePlatformsHint')}</p>
        <div className="flex flex-wrap gap-4">
          {PLATFORMS.map((p) => (
            <label key={p} className="flex items-center gap-2 text-sm text-foreground">
              <Checkbox
                checked={draft.imagePlatforms.includes(p)}
                onCheckedChange={(on) => togglePlatform(p, on === true)}
              />
              <span className="font-mono">{p}</span>
            </label>
          ))}
        </div>
        <Input
          value={draft.imageRepository}
          aria-label={t('agentReleases.settings.imageRepository')}
          placeholder={t('agentReleases.settings.imageRepository')}
          onChange={(e) => patch({ imageRepository: e.target.value })}
        />
      </div>
      <div className="flex justify-end gap-2">
        <Button variant="outline" disabled={!dirty || save.isPending} onClick={() => setDraft(toDraft(settings))}>
          {t('actions.cancel', { ns: 'common' })}
        </Button>
        <Button
          permission={PERM.UPDATES_MANAGE}
          disabled={!dirty || save.isPending}
          onClick={() => save.mutate(fromDraft(draft))}
        >
          {t('actions.save', { ns: 'common' })}
        </Button>
      </div>
    </div>
  );
}

export function AgentReleaseSettingsCard() {
  const { t } = useTranslation('settings');
  const { data } = useAgentReleases();
  if (!data) return null;
  return (
    <div className="rounded-lg border border-border p-5">
      <div className="mb-4 flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <IconCloudDownload className="size-4" />
        </span>
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('agentReleases.settings.title')}</h3>
          <p className="text-xs text-muted-foreground">{t('agentReleases.settings.subtitle')}</p>
        </div>
      </div>
      <ReleaseSettingsForm
        key={JSON.stringify(data.settings)}
        settings={data.settings}
        nextCheckAt={data.state.nextCheckAt}
      />
    </div>
  );
}

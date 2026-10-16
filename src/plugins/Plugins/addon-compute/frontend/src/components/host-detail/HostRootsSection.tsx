// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { Input } from '@resources/components/ui/Input';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { StatusError } from '@resources/components/ui/StatusError';
import { COMPUTE_PERM } from '../../permissions';
import {
  useApplyLocations,
  useEnvironmentLocations,
  useRemoveLocations,
  useSaveLocations,
} from '../../hooks/useLocations';
import type { LocationSource, OverviewEntry } from '../../types';

interface HostRootsSectionProps {
  entry: OverviewEntry;
}

type Draft = { vmPath: string; diskPath: string; imagePath: string };

const FIELDS: { key: keyof Draft; sourceOf: (view: RootSources) => LocationSource }[] = [
  { key: 'vmPath', sourceOf: (view) => view.vmSource },
  { key: 'diskPath', sourceOf: (view) => view.diskSource },
  { key: 'imagePath', sourceOf: (view) => view.imageSource },
];

type RootSources = {
  vmSource: LocationSource;
  diskSource: LocationSource;
  imageSource: LocationSource;
};

export function HostRootsSection({ entry }: HostRootsSectionProps) {
  const { t } = useTranslation('compute');
  const { data, isLoading, dataUpdatedAt } = useEnvironmentLocations(entry.environmentId);
  const save = useSaveLocations();
  const remove = useRemoveLocations();
  const apply = useApplyLocations();

  const [draft, setDraft] = useState<Draft>({ vmPath: '', diskPath: '', imagePath: '' });

  const [seededAt, setSeededAt] = useState<number | null>(null);
  if (data && seededAt !== dataUpdatedAt) {
    setSeededAt(dataUpdatedAt);
    setDraft({
      vmPath: data.stored.vmPath ?? '',
      diskPath: data.stored.diskPath ?? '',
      imagePath: data.stored.imagePath ?? '',
    });
  }

  if (isLoading || !data) {
    return (
      <Card className="space-y-3 p-5">
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-24 w-full" />
      </Card>
    );
  }

  const effective = data.effective;
  const busy = save.isPending || remove.isPending || apply.isPending;

  return (
    <Card className="space-y-4 p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h2 className="text-sm font-medium text-foreground">{t('hosts.roots.title')}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('hosts.roots.hint')}</p>
        </div>
        <div className="flex items-center gap-2">
          {!effective.applied && (
            <Badge variant="destructive" className="px-1.5 py-0 text-[10px]">
              {t('hosts.roots.notApplied')}
            </Badge>
          )}
          <StatusError error={effective.lastError} />
        </div>
      </div>

      <div className="space-y-3">
        {FIELDS.map((field) => (
          <div key={field.key} className="space-y-1.5">
            <div className="flex flex-wrap items-center gap-2">
              <label className="text-sm text-foreground" htmlFor={`compute-root-${field.key}-${entry.environmentId}`}>
                {t(`hosts.roots.${field.key}`)}
              </label>
              <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                {t(`hosts.roots.source.${field.sourceOf(effective)}`)}
              </Badge>
            </div>
            <Input
              id={`compute-root-${field.key}-${entry.environmentId}`}
              value={draft[field.key]}
              disabled={busy}
              placeholder={effective[field.key] || t('hosts.roots.inherited')}
              onChange={(ev) => setDraft((current) => ({ ...current, [field.key]: ev.target.value }))}
            />
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          permission={COMPUTE_PERM.MANAGE}
          disabled={busy}
          onClick={() => save.mutate({ envId: entry.environmentId, body: { scopeId: entry.environmentId, ...draft } })}
        >
          {t('hosts.roots.save')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          permission={COMPUTE_PERM.DELETE}
          disabled={busy}
          onClick={() => remove.mutate(entry.environmentId)}
        >
          {t('hosts.roots.reset')}
        </Button>
        {!effective.applied && (
          <Button
            size="sm"
            variant="outline"
            permission={COMPUTE_PERM.MANAGE}
            disabled={busy}
            onClick={() => apply.mutate(entry.environmentId)}
          >
            <IconRefresh className="size-4" aria-hidden="true" />
            {t('hosts.roots.retry')}
          </Button>
        )}
      </div>
    </Card>
  );
}

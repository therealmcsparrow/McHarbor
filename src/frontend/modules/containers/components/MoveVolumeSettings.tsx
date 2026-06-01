// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { MoveContainerPlan, MoveVolumeConfig } from '../hooks/useContainers';

type MoveVolumeSettingsProps = {
  plan: MoveContainerPlan;
  volumes: MoveVolumeConfig[];
  onVolumesChange: (value: MoveVolumeConfig[]) => void;
};

export function moveVolumeConfigsFromPlan(plan: MoveContainerPlan): MoveVolumeConfig[] {
  return plan.volumes
    .filter((volume) => volume.type === 'volume' || volume.type === 'bind')
    .map((volume) => ({
      sourceName: volume.name ?? '',
      sourceDestination: volume.destination,
      targetName: volume.targetName ?? volume.name ?? '',
      targetSource: volume.targetSource || volume.source,
      targetDestination: volume.targetDestination || volume.destination,
    }));
}

export function MoveVolumeSettings({ plan, volumes, onVolumesChange }: MoveVolumeSettingsProps) {
  const { t } = useTranslation('containers');
  const volumePlans = plan.volumes.filter((volume) => volume.type === 'volume' || volume.type === 'bind');

  if (volumePlans.length === 0) {
    return null;
  }

  const sourceKey = (sourceName: string | undefined, sourceDestination: string) => (
    sourceName ? `name:${sourceName}` : `destination:${sourceDestination}`
  );

  const updateVolume = (sourceName: string, sourceDestination: string, patch: Partial<MoveVolumeConfig>) => {
    const key = sourceKey(sourceName, sourceDestination);
    const existing = volumes.find((volume) => sourceKey(volume.sourceName, volume.sourceDestination ?? '') === key);
    if (!existing) {
      const source = volumePlans.find((volume) => sourceKey(volume.name, volume.destination) === key);
      onVolumesChange([
        ...volumes,
        {
          sourceName,
          sourceDestination: source?.destination,
          targetName: source?.targetName ?? source?.name ?? '',
          targetSource: source?.targetSource || source?.source,
          targetDestination: source?.targetDestination || source?.destination,
          ...patch,
        },
      ]);
      return;
    }
    onVolumesChange(volumes.map((volume) => (
      sourceKey(volume.sourceName, volume.sourceDestination ?? '') === key ? { ...volume, ...patch } : volume
    )));
  };

  return (
    <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-3">
      <div className="flex items-center gap-2 text-sm font-medium">
        <IconDatabase className="size-4 text-lime-400" />
        {t('moveDialog.editVolumeMounts')}
      </div>

      <div className="space-y-3">
        {volumePlans.map((source) => {
          const key = sourceKey(source.name, source.destination);
          const volume = volumes.find((item) => sourceKey(item.sourceName, item.sourceDestination ?? '') === key) ?? {
            sourceName: source.name ?? '',
            sourceDestination: source.destination,
            targetName: source.targetName ?? source.name ?? '',
            targetSource: source.targetSource || source.source,
            targetDestination: source.targetDestination || source.destination,
          };
          const isBind = source.type === 'bind';
          const inputSourceId = `move-volume-source-${key}`;
          const inputPathId = `move-volume-path-${key}`;

          return (
            <div key={`${source.name}-${source.destination}`} className="space-y-3 rounded-md border border-border bg-background/50 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="min-w-0 text-sm font-medium">
                  <span className="truncate">{source.name || source.source || source.destination}</span>
                  <span className="ml-2 text-xs text-muted-foreground">{source.destination}</span>
                </div>
                <Badge variant={source.manual ? 'warning' : source.willCreate ? 'warning' : 'secondary'}>
                  {source.manual ? t('moveDialog.manual') : source.willCreate ? t('moveDialog.create') : t('moveDialog.keep')}
                </Badge>
              </div>

              <div className="grid gap-3 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor={inputSourceId}>
                    {isBind ? t('moveDialog.targetHostPath') : t('moveDialog.targetVolume')}
                  </Label>
                  <Input
                    id={inputSourceId}
                    value={isBind ? volume.targetSource ?? '' : volume.targetName ?? ''}
                    onChange={(event) => updateVolume(
                      volume.sourceName,
                      volume.sourceDestination ?? source.destination,
                      isBind ? { targetSource: event.target.value } : { targetName: event.target.value },
                    )}
                    placeholder={isBind ? source.source : source.name}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor={inputPathId}>{t('moveDialog.targetContainerPath')}</Label>
                  <Input
                    id={inputPathId}
                    value={volume.targetDestination ?? ''}
                    onChange={(event) => updateVolume(
                      volume.sourceName,
                      volume.sourceDestination ?? source.destination,
                      { targetDestination: event.target.value },
                    )}
                    placeholder={source.destination}
                  />
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}

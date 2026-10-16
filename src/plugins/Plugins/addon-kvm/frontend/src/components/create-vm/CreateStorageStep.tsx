// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconBolt } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Spinner } from '@resources/components/ui/Spinner';
import { DISK_BUSES } from '../../diskBus';
import { formatBytes } from '../../format';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';
import type { DiskMode } from './model';

const DISK_MODES: DiskMode[] = ['new', 'existing'];

export function CreateStorageStep({ form }: { form: CreateVMForm }) {
  const { t } = useTranslation('vm');
  const {
    diskGb,
    setDiskGb,
    setDiskBus,
    poolId,
    setPoolId,
    diskMode,
    setDiskMode,
    volumePath,
    setVolumePath,
    pools,
    dataLocation,
    pool,
    freeVolumes,
    volumesPending,
    volumesFailed,
    hasPools,
    canAdopt,
    adopting,
    osFamily,
    effectiveDiskBus,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.storage.title')}</h3>

      {hasPools ? (
        <div>
          <Label>{t('create.storage.pool')}</Label>
          <Select
            value={poolId}
            ariaLabel={t('create.storage.pool')}
            options={(pools ?? []).map((entry) => ({
              value: entry.id,
              label: `${entry.name} (${entry.backend})`,
            }))}
            onChange={setPoolId}
          />
          {pool?.instant && (
            <Badge variant="success" className="mt-2">
              <IconBolt className="h-3 w-3" aria-hidden="true" />
              {t('create.storage.instantNote')}
            </Badge>
          )}
        </div>
      ) : (
        <div>
          <Label htmlFor="vm-data-location">{t('create.storage.dataLocation')}</Label>
          <p
            id="vm-data-location"
            className="mt-1 truncate rounded-md border border-border bg-muted/40 px-3 py-2 font-mono text-xs text-foreground"
            title={dataLocation}
          >
            {dataLocation}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">{t('create.storage.dataLocationHint')}</p>
        </div>
      )}

      {canAdopt && (
        <fieldset aria-label={t('create.storage.mode')} className="m-0 min-w-0 space-y-2 border-0 p-0">
          <Label>{t('create.storage.mode')}</Label>
          <div className="grid gap-2 sm:grid-cols-2">
            {DISK_MODES.map((entry) => (
              <Button
                key={entry}
                variant="outline"
                aria-pressed={diskMode === entry}
                onClick={() => setDiskMode(entry)}
                className={cn(
                  'h-auto flex-col items-start gap-1 p-3 text-left whitespace-normal',
                  diskMode === entry && 'border-primary bg-primary/5',
                )}
              >
                <span className="text-sm font-medium text-foreground">
                  {entry === 'new' ? t('create.storage.modeNew') : t('create.storage.modeExisting')}
                </span>
                <span className="text-xs font-normal text-muted-foreground">
                  {entry === 'new' ? t('create.storage.modeNewHint') : t('create.storage.modeExistingHint')}
                </span>
              </Button>
            ))}
          </div>
        </fieldset>
      )}

      {adopting ? (
        <fieldset>
          <Label>{t('create.storage.existing')}</Label>
          {volumesPending ? (
            <Spinner className="h-4 w-4" />
          ) : volumesFailed ? (
            <p className="text-sm text-warning">{t('create.storage.existingFailed')}</p>
          ) : (freeVolumes ?? []).length === 0 ? (
            <p className="text-sm text-warning">{t('create.storage.noExisting')}</p>
          ) : (
            <Select
              value={volumePath}
              ariaLabel={t('create.storage.existing')}
              options={(freeVolumes ?? []).map((entry) => ({
                value: entry.volumePath,
                label: t('create.storage.existingLabel', {
                  name: entry.name,
                  size: formatBytes(entry.sizeBytes ?? 0),
                }),
              }))}
              onChange={setVolumePath}
            />
          )}
        </fieldset>
      ) : (
        <div>
          <Label htmlFor="vm-disk">{t('create.storage.size')}</Label>
          <NumberInput id="vm-disk" min={1} value={diskGb} onChange={setDiskGb} />
          <p className="mt-1 text-xs text-muted-foreground">{t('create.storage.thinNote')}</p>
        </div>
      )}

      <div>
        <Label>{t('create.storage.bus')}</Label>
        <Select
          value={effectiveDiskBus}
          ariaLabel={t('create.storage.bus')}
          options={DISK_BUSES.map((entry) => ({
            value: entry,
            label: entry,
          }))}
          onChange={setDiskBus}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.storage.busHint')}</p>
        {osFamily === 'windows' && effectiveDiskBus.startsWith('virtio') && (
          <p className="mt-1 text-xs text-warning">{t('hardware.addDisk.virtioWindows')}</p>
        )}
      </div>

      {/*
        A pool with less free space than the disk asked for still works
        — the disk is thin — but it is the shape that ends in a paused
        guest later, so it is said now.
      */}
      {!adopting && pool && pool.capacityBytes - pool.usedBytes < diskGb * 1024 ** 3 && (
        <div className="flex items-start gap-2 rounded-md bg-warning/10 p-3">
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" aria-hidden="true" />
          <p className="text-sm text-foreground">
            {t('create.storage.capacityWarning', {
              free: formatBytes(pool.capacityBytes - pool.usedBytes),
            })}
          </p>
        </div>
      )}
    </>
  );
}

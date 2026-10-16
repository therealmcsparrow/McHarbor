// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { formatBytes } from '../../format';
import type { CreateVMForm } from '../../hooks/useCreateVMForm';
import { TemplateSummary } from '../CreateFromTemplate';

export function CreateSummaryStep({ form }: { form: CreateVMForm }) {
  const { t } = useTranslation('vm');
  const {
    nameValue,
    source,
    vcpus,
    memoryMb,
    diskGb,
    autostart,
    setAutostart,
    linked,
    template,
    dataLocation,
    pool,
    hasPools,
    network,
    adopting,
    volume,
    image,
    disc,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.summary.title')}</h3>
      {source === 'template' ? (
        <TemplateSummary name={nameValue} template={template} linked={linked} />
      ) : (
        <dl className="grid gap-3 sm:grid-cols-2">
          <Fact label={t('list.columns.name')} value={nameValue} />
          <Fact
            label={
              source === 'iso'
                ? t('create.source.isoLabel')
                : source === 'blank'
                  ? t('create.steps.source')
                  : t('create.source.image')
            }
            value={
              source === 'blank'
                ? t('create.summary.noSource')
                : ((source === 'iso' ? disc?.displayName : image?.displayName) ?? '—')
            }
          />
          <Fact label={t('create.steps.resources')} value={`${vcpus} × ${Math.round(memoryMb / 1024)} GB`} />
          {adopting ? (
            <Fact
              label={t('create.summary.existingDisk')}
              value={volume ? `${volume.name} — ${formatBytes(volume.sizeBytes ?? 0)}` : '—'}
            />
          ) : (
            <Fact label={t('create.storage.size')} value={`${diskGb} GB`} />
          )}
          <Fact
            label={hasPools ? t('create.storage.pool') : t('create.storage.dataLocation')}
            value={hasPools ? (pool?.name ?? '—') : dataLocation}
          />
          <Fact label={t('create.network.network')} value={network?.name ?? '—'} />
        </dl>
      )}

      <div className="flex items-start justify-between gap-3">
        <Label htmlFor="vm-autostart" className="text-sm font-normal">
          {t('create.summary.createAndStart')}
        </Label>
        <Switch id="vm-autostart" checked={autostart} onCheckedChange={setAutostart} />
      </div>
    </>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 text-sm text-foreground">{value}</dd>
    </div>
  );
}

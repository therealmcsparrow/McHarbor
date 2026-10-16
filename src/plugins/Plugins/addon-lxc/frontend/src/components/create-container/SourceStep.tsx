// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { CreateContainerForm } from './useCreateContainerForm';

/** An example hostname shown as the name field placeholder; not translatable copy. */
const CONTAINER_NAME_EXAMPLE = 'web-01';

export function SourceStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const {
    environments,
    environmentId,
    setEnvironmentId,
    ctName,
    setCtName,
    imageId,
    setImageId,
    poolId,
    setPoolId,
    poolBackend,
    setPoolBackend,
    poolTarget,
    setPoolTarget,
  } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.source.title')}</h3>
      {environments.length > 1 && (
        <div>
          <Label htmlFor="ct-env">{t('list.columns.host')}</Label>
          <Select
            value={environmentId}
            ariaLabel={t('list.columns.host')}
            options={environments.map((env) => ({ value: env.id, label: env.name }))}
            onChange={setEnvironmentId}
          />
        </div>
      )}
      <div>
        <Label htmlFor="ct-name">{t('list.columns.name')}</Label>
        <Input
          id="ct-name"
          value={ctName}
          onChange={(ev) => setCtName(ev.target.value)}
          placeholder={CONTAINER_NAME_EXAMPLE}
          autoComplete="off"
          spellCheck={false}
        />
      </div>
      <div>
        <Label htmlFor="ct-image">{t('create.source.image')}</Label>
        <Input id="ct-image" value={imageId} onChange={(ev) => setImageId(ev.target.value)} />
        <p className="mt-1 text-xs text-muted-foreground">{t('create.source.imageHint')}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        <div>
          <Label htmlFor="ct-pool">{t('list.columns.pool')}</Label>
          <Input id="ct-pool" value={poolId} onChange={(ev) => setPoolId(ev.target.value)} />
        </div>
        <div>
          <Label htmlFor="ct-pool-backend">{t('create.source.title')}</Label>
          <Input id="ct-pool-backend" value={poolBackend} onChange={(ev) => setPoolBackend(ev.target.value)} />
        </div>
        <div>
          <Label htmlFor="ct-pool-target">{t('detail.overview.pool')}</Label>
          <Input id="ct-pool-target" value={poolTarget} onChange={(ev) => setPoolTarget(ev.target.value)} />
        </div>
      </div>
    </>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Checkbox } from '@resources/components/ui/Checkbox';
import { Label } from '@resources/components/ui/Label';
import type { CreateContainerForm } from './useCreateContainerForm';

export function SummaryStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const { ctName, environmentId, initSystem, privileged, memoryMb, pidsMax, startAfterCreate, setStartAfterCreate } =
    form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.summary.title')}</h3>
      <dl className="grid gap-3 sm:grid-cols-2">
        <Fact label={t('list.columns.name')} value={ctName} />
        <Fact label={t('list.columns.host')} value={environmentId} />
        <Fact label={t('create.profile.init')} value={initSystem} />
        <Fact
          label={t('list.columns.isolation')}
          value={privileged ? t('isolation.privileged') : t('isolation.unprivileged')}
        />
        <Fact label={t('resources.memoryMax')} value={`${memoryMb} MB`} />
        <Fact label={t('resources.pidsMax')} value={`${pidsMax}`} />
      </dl>

      <div className="flex items-start gap-2">
        <Checkbox
          id="ct-start"
          checked={startAfterCreate}
          onCheckedChange={(checked) => setStartAfterCreate(checked === true)}
        />
        <div>
          <Label htmlFor="ct-start">{t('create.summary.startAfterCreate')}</Label>
          <p className="text-xs text-muted-foreground">{t('create.summary.startHint')}</p>
        </div>
      </div>
    </>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 text-sm text-foreground">{value || '—'}</dd>
    </div>
  );
}

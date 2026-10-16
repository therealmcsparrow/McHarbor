// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import type { CreateContainerForm } from './useCreateContainerForm';

export function ResourcesStep({ form }: { form: CreateContainerForm }) {
  const { t } = useTranslation('ct');
  const { preset, setPreset, applyPreset, memoryMb, setMemoryMb, cpuMax, setCpuMax, pidsMax, setPidsMax } = form;

  return (
    <>
      <h3 className="font-medium text-foreground">{t('create.resources.title')}</h3>
      <div>
        <Label htmlFor="ct-preset">{t('create.resources.preset')}</Label>
        <Select
          value={preset}
          ariaLabel={t('create.resources.preset')}
          options={[
            { value: 'small', label: t('create.resources.presets.small') },
            { value: 'medium', label: t('create.resources.presets.medium') },
            { value: 'large', label: t('create.resources.presets.large') },
            { value: 'custom', label: t('create.resources.presets.custom') },
          ]}
          onChange={applyPreset}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        <div>
          <Label htmlFor="ct-memory">{t('resources.memoryMax')}</Label>
          <NumberInput
            id="ct-memory"
            min={64}
            value={memoryMb}
            onChange={(next) => {
              setMemoryMb(next);
              setPreset('custom');
            }}
          />
        </div>
        <div>
          <Label htmlFor="ct-cpu">{t('resources.cpuMax')}</Label>
          <Input
            id="ct-cpu"
            value={cpuMax}
            onChange={(ev) => {
              setCpuMax(ev.target.value);
              setPreset('custom');
            }}
          />
        </div>
        <div>
          <Label htmlFor="ct-pids">{t('resources.pidsMax')}</Label>
          <NumberInput id="ct-pids" min={1} value={pidsMax} onChange={setPidsMax} />
        </div>
      </div>
      {/*
        The reassurance that makes this step quick to get past: none of
        these is a commitment, because all of them can be changed later
        without stopping the container.
      */}
      <p className="text-xs text-muted-foreground">{t('create.resources.liveNote')}</p>
    </>
  );
}

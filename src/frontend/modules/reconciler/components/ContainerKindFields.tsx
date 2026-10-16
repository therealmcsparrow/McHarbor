// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { useContainers } from '@resources/hooks/useContainers';
import { useImages } from '@resources/hooks/useImages';
import type { CreateDesiredStateInput } from '../hooks/useReconciler';
import { PickOrType } from './PickOrType';

type Props = {
  form: CreateDesiredStateInput;
  set: (patch: Partial<CreateDesiredStateInput>) => void;
};

const containerName = (names: string[]) => (names[0] ?? '').replace(/^\//, '');

export function ContainerKindFields({ form, set }: Props) {
  const { t } = useTranslation('common');
  const containers = useContainers(true);
  const images = useImages();

  const containerOptions = useMemo(
    () =>
      (containers.data ?? [])
        .map((c) => ({
          value: containerName(c.Names),
          label: containerName(c.Names),
          description: `${c.State} · ${c.Image}`,
        }))
        .filter((o) => o.value !== '')
        .sort((a, b) => a.label.localeCompare(b.label)),
    [containers.data],
  );
  const imageOptions = useMemo(() => {
    const refs = new Set<string>();
    for (const img of images.data ?? [])
      for (const tag of img.RepoTags ?? []) if (tag !== '<none>:<none>') refs.add(tag);
    for (const c of containers.data ?? []) if (c.Image && !c.Image.startsWith('sha256:')) refs.add(c.Image);
    return [...refs].sort().map((r) => ({ value: r, label: r }));
  }, [images.data, containers.data]);

  const pickContainer = (itemName: string) => {
    const current = (containers.data ?? []).find((c) => containerName(c.Names) === itemName);
    set({
      containerName: itemName,
      ...(current && !form.imageRef ? { imageRef: current.Image } : {}),
      ...(current ? { desiredStatus: current.State === 'running' ? 'running' : 'stopped' } : {}),
    });
  };

  return (
    <>
      <div>
        <Label className="mb-2">{t('reconciler.containerNameLabel')}</Label>
        <PickOrType
          value={form.containerName}
          onChange={pickContainer}
          options={containerOptions}
          ariaLabel={t('reconciler.containerNameLabel')}
          placeholder={t('pick.container', { ns: 'reconciler' })}
          manualPlaceholder={t('placeholders.containerName')}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('pick.containerHint', { ns: 'reconciler' })}</p>
      </div>
      <div>
        <Label className="mb-2">{t('reconciler.imageRefLabel')}</Label>
        <PickOrType
          value={form.imageRef}
          onChange={(imageRef) => set({ imageRef })}
          options={imageOptions}
          ariaLabel={t('reconciler.imageRefLabel')}
          placeholder={t('pick.image', { ns: 'reconciler' })}
          manualPlaceholder={t('placeholders.image')}
        />
      </div>
    </>
  );
}

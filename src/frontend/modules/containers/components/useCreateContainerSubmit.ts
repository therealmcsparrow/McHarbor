// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { toastError } from '@resources/utils/toast';
import { useImages, usePullImage } from '@resources/hooks/useImages';
import { useBatchProgressOperation } from '@resources/hooks/useBatchProgressOperation';
import { useCreateContainer, useStartContainer } from '../hooks/useCreateContainer';
import { buildPayloadFromForm } from './createContainerPayload';
import type { CreateContainerForm } from './useCreateContainerForm';

export function useCreateContainerSubmit(
  form: CreateContainerForm,
  onOpenChange: (open: boolean) => void,
  onCreated?: (id: string, name: string) => void,
) {
  const { t } = useTranslation('containers');
  const create = useCreateContainer();
  const start = useStartContainer();
  const pullImageMutation = usePullImage();
  const imagesQuery = useImages();
  const restoreProgress = useBatchProgressOperation();
  const createProgress = useBatchProgressOperation();
  const queryClient = useQueryClient();

  // The Docker daemon answers 500 "No such image" when a container is created
  // from an image it does not have, so the pull only runs for refs not cached.
  function imageIsLocal(ref: string): boolean {
    const list = imagesQuery.data ?? [];
    const target = ref.trim();
    if (!target) return false;
    return list.some((img) => (img.RepoTags ?? []).some((tag) => tag === target || tag.startsWith(target + ':')));
  }

  async function handleCreate() {
    const result = buildPayloadFromForm(form, t);
    if ('error' in result) {
      create.reset();
      toast.error(result.error);
      return;
    }
    const payload = result;
    const imageRef = payload.image;
    const containerName = payload.name || imageRef;
    const needsPull = form.pullImage && !!imageRef && !imageIsLocal(imageRef);
    const needsStart = form.autoStart;

    try {
      // Captured by `execute` so `onComplete` can hand the new id back to the
      // caller; the batch result only forwards the item, not the execute return.
      let createdContainerId: string | undefined;
      await createProgress.runBatchOperation({
        title: t('create.progress.title'),
        description: t('create.progress.description', { name: containerName }),
        actionLabel: t('create.progress.action'),
        getKey: (item) => item.key,
        getLabel: (item) => item.label,
        execute: async (_item, _ctx) => {
          if (needsPull) {
            await pullImageMutation.mutateAsync(imageRef);
          }
          const created = await create.mutateAsync(payload);
          createdContainerId = created.id;
          if (needsStart) {
            await start.mutateAsync(created.id);
          }
          void queryClient.invalidateQueries({ queryKey: ['containers'] });
        },
        items: [
          {
            key: containerName,
            label: containerName,
          },
        ],
        onComplete: (resultValue) => {
          if (resultValue.successfulItems.length === 0 || !createdContainerId) return;
          toast.success(t('create.toast.created', { name: containerName }));
          onOpenChange(false);
          if (onCreated) {
            onCreated(createdContainerId, containerName);
          }
        },
      });
    } catch (err) {
      toastError(err);
    }
  }

  const isBusy =
    create.isPending ||
    start.isPending ||
    pullImageMutation.isPending ||
    restoreProgress.isRunning ||
    createProgress.isRunning;
  const canApply = !isBusy && form.image.trim().length > 0 && form.entryName.trim().length > 0;

  return { create, createProgress, restoreProgress, handleCreate, isBusy, canApply };
}

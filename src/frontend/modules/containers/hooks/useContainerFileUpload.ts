// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useUploadActivityStore } from '@resources/stores/upload-activity';
import { postFormWithProgress } from '@resources/hooks/containerBackups/uploadWithProgress';
import { toast } from 'sonner';
import { useTranslation } from 'react-i18next';

export function useUploadFile(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');
  const [progress, setProgress] = useState<number | null>(null);
  const startUpload = useUploadActivityStore((state) => state.startUpload);
  const finishUpload = useUploadActivityStore((state) => state.finishUpload);

  const mutation = useMutation({
    mutationFn: async ({
      path,
      files,
      directories = [],
    }: {
      path: string;
      files: Array<{ file: File; path: string }>;
      directories?: string[];
    }) => {
      const params = new URLSearchParams({ path });
      if (envId) params.set('env', envId);
      const form = new FormData();
      files.forEach((item) => {
        form.append('paths', item.path);
        form.append('files', item.file, item.file.name);
      });
      directories.forEach((dir) => form.append('dirs', dir));

      const data = await postFormWithProgress<unknown>(
        `/api/containers/${containerId}/files/upload?${params}`,
        form,
        ({ loaded, total }) => setProgress(total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : null),
      );
      setProgress(100);
      return data;
    },
    onMutate: async () => {
      setProgress(0);
      startUpload(envId);
      await Promise.all([
        queryClient.cancelQueries({ queryKey: ['containers'] }),
        queryClient.cancelQueries({ queryKey: ['containers-bulk-stats', envId] }),
        queryClient.cancelQueries({ queryKey: ['host-metrics', envId] }),
        queryClient.cancelQueries({ queryKey: ['docker-info', envId] }),
        queryClient.cancelQueries({ queryKey: ['dashboard-stats', envId] }),
        queryClient.cancelQueries({ queryKey: ['environment-update-summary', envId] }),
      ]);
    },
    onSuccess: (_data, vars) => {
      toast.success(t('files.toast.uploaded', { count: vars.files.length }));
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
    meta: { error: t('files.toast.uploadFailed') },
    onSettled: () => {
      finishUpload(envId);
      window.setTimeout(() => setProgress(null), 600);
      window.setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: ['containers'] });
        void queryClient.invalidateQueries({ queryKey: ['containers-bulk-stats', envId] });
        void queryClient.invalidateQueries({ queryKey: ['host-metrics', envId] });
        void queryClient.invalidateQueries({ queryKey: ['docker-info', envId] });
      }, 800);
    },
  });

  return { ...mutation, progress, resetProgress: () => setProgress(null) };
}

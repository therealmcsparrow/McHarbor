// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ImageInfo, ImageInspect, ImageHistoryItem } from '@core/types/docker';

export function useImages() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['images', envId],
    queryFn: () =>
      api
        .get<ImageInfo[]>('/images', envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });
}

export function useImage(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['image', envId, id],
    queryFn: () =>
      api
        .get<ImageInspect>(`/images/${id}`, envId ? { env: envId } : {})
        .then((r) => r.data),
    enabled: !!id,
  });
}

export function useImageHistory(id: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['image-history', envId, id],
    queryFn: () =>
      api
        .get<ImageHistoryItem[]>(`/images/${id}/history`, envId ? { env: envId } : {})
        .then((r) => r.data ?? []),
    enabled: !!id,
  });
}

export function usePullImage() {
  const { t } = useTranslation('images');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (image: string) =>
      api.post(`/images/pull${envId ? `?env=${envId}` : ''}`, { image }).then(assertSuccess),
    meta: { success: t('toast.pulled') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

export function useRemoveImage() {
  const { t } = useTranslation('images');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: (id: string) =>
      api.del(`/images/${id}`, envId ? { env: envId } : {}).then(assertSuccess),
    meta: { success: t('toast.removed') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

export function usePruneImages() {
  const { t } = useTranslation('images');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: () =>
      api.post(`/images/prune${envId ? `?env=${envId}` : ''}`, {}).then(assertSuccess),
    meta: { success: t('toast.pruned') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

export function useExportImage() {
  const envId = useEnvironmentStore((s) => s.currentId);

  const exportImage = (id: string, name: string) => {
    const params = new URLSearchParams();
    if (envId) params.set('env', envId);
    const url = `/api/images/${encodeURIComponent(id)}/export?${params}`;

    const a = document.createElement('a');
    a.href = url;
    a.download = name.replace(/[/:]/g, '_') + '.tar';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  };

  return { exportImage };
}

export function useImportImage() {
  const { t } = useTranslation('images');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);

  return useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData();
      formData.append('file', file);
      const params = new URLSearchParams();
      if (envId) params.set('env', envId);

      const stored = localStorage.getItem('mcharbor-language');
      let lang = 'en';
      if (stored) {
        try {
          lang = JSON.parse(stored)?.state?.language || 'en';
        } catch {
          lang = stored;
        }
      }

      const res = await fetch(`/api/images/import?${params}`, {
        method: 'POST',
        body: formData,
        credentials: 'include',
        headers: { 'Accept-Language': lang },
      });

      if (!res.ok) {
        const data = await res.json().catch(() => null);
        throw new Error(data?.error || 'Import failed');
      }

      return res.json();
    },
    meta: { success: t('toast.imported') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['images'] });
    },
  });
}

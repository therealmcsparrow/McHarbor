// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api, type ApiResponse } from '@core/api/client';
import { apiErrorOf } from '@resources/utils/api-mutation';
import { hostToast } from '../hostToast';
import type { ComputeImage, ImageLibrary } from '../types';
import { ComputeRequestError, KEYS, params, unwrap } from './computeBase';

/** The image catalog, with per-host download state when scoped. */
export function useImages(envId?: string, kind?: string) {
  return useQuery({
    queryKey: KEYS.images(envId, kind),
    queryFn: async () => unwrap(await api.get<ComputeImage[]>('/compute/images', params({ env: envId, kind }))) ?? [],
    /**
     * Polled only while something is downloading. A catalog nobody is
     * downloading into does not change on its own, and a fixed interval would
     * be a request every couple of seconds for a static list.
     */
    refetchInterval: (query) => {
      const images = query.state.data as ComputeImage[] | undefined;
      const busy = images?.some((image) => image.cache?.state === 'downloading' || image.cache?.state === 'pending');
      return busy ? 2_000 : false;
    },
  });
}

export function useImageLibrary() {
  return useQuery({
    queryKey: KEYS.imageLibrary,
    queryFn: async () => unwrap(await api.get<ImageLibrary>('/compute/images/library')),
    staleTime: 5 * 60_000,
  });
}

export function useUploadImage() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');
  return useMutation({
    meta: { success: t('msg.compute.image_uploaded') },
    mutationFn: async (input: {
      file: File;
      kind: string;
      displayName: string;
      distro: string;
      release: string;
      arch: string;
      variant?: string;
      initSystem?: string;
      onProgress?: (percent: number) => void;
    }) => {
      const { file, onProgress, ...meta } = input;

      const form = new FormData();
      for (const [key, value] of Object.entries(meta)) {
        if (value) form.append(key, String(value));
      }
      form.append('fileName', file.name);
      form.append('file', file);

      return new Promise<ComputeImage>((resolve, reject) => {
        const request = new XMLHttpRequest();
        request.open('POST', '/api/compute/images/upload');
        request.withCredentials = true;

        request.upload.onprogress = (ev) => {
          if (ev.lengthComputable && onProgress) {
            onProgress(Math.round((ev.loaded / ev.total) * 100));
          }
        };
        request.onerror = () => reject(new ComputeRequestError('err.compute.upload_failed'));
        request.onabort = () => reject(new ComputeRequestError('err.compute.upload_failed'));
        request.onload = () => {
          try {
            const body = JSON.parse(request.responseText) as ApiResponse<ComputeImage>;
            if (!body.success) {
              reject(new ComputeRequestError(body.code ?? body.error, apiErrorOf(body).details));
              return;
            }
            resolve(body.data as ComputeImage);
          } catch {
            reject(new ComputeRequestError('err.compute.upload_failed'));
          }
        };
        request.send(form);
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.imageLibrary });
    },
  });
}

export function useAddImage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: Record<string, unknown>) => unwrap(await api.post<ComputeImage>('/compute/images', body)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
    },
  });
}

export function useFetchImageEverywhere() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (imageId: string) =>
      unwrap(
        await api.post<{
          imageId: string;
          started?: string[];
          sharedWith?: string[];
          skipped?: Record<string, string>;
        }>(`/compute/images/${imageId}/fetch-all`, {}),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
    },
  });
}

export function useReloadCatalog() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () =>
      unwrap(
        await api.post<{ dir: string; loaded: number; removed: number; skipped?: string[] }>(
          '/compute/images/catalog/reload',
          {},
        ),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
    },
  });
}

export function useFetchImage() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');
  return useMutation({
    meta: { success: t('msg.compute.fetch_started') },
    mutationFn: async (input: { imageId: string; environmentId: string; acknowledgeUnverified?: boolean }) => {
      const { imageId, ...body } = input;
      return unwrap(await api.post<unknown>(`/compute/images/${imageId}/fetch`, body));
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

export function useImageFetchWatcher(images: ComputeImage[] | undefined) {
  const { t } = useTranslation('compute');
  const seen = useRef(new Map<string, string>());

  useEffect(() => {
    if (!images) return;
    const next = new Map<string, string>();
    for (const image of images) {
      const cache = image.cache;
      if (!cache) continue;
      const key = `${image.id}:${cache.environmentId}`;
      next.set(key, cache.state);
      const before = seen.current.get(key);
      if (before !== 'downloading' && before !== 'pending') continue;
      if (cache.state === 'ready') {
        hostToast.success(t('msg.compute.image_fetched'));
      } else if (cache.state === 'failed') {
        hostToast.error(
          cache.lastError
            ? t('err.compute.image_fetch_failed', { error: cache.lastError })
            : t('err.compute.image_fetch_failed_unknown'),
        );
      }
    }
    seen.current = next;
  }, [images, t]);
}

export function useEvictImage() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('compute');
  return useMutation({
    meta: { success: t('msg.compute.image_evicted') },
    mutationFn: async (input: { imageId: string; environmentId: string }) =>
      unwrap(
        await api.del<null>(`/compute/images/${input.imageId}/cache`, {
          env: input.environmentId,
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
      void queryClient.invalidateQueries({ queryKey: KEYS.overview });
    },
  });
}

export function useDeleteImage() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (imageId: string) => unwrap(await api.del<null>(`/compute/images/${imageId}`)),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compute', 'images'] });
    },
  });
}

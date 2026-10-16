// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { ApiError, assertSuccess, describeApiError } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';
import type { FileEntry } from '@core/types/docker';
import { toast } from 'sonner';
import { useTranslation } from 'react-i18next';

export { useUploadFile } from './useContainerFileUpload';

// The raw-body endpoints bypass the api client, so a refusal must be unwrapped here or the operator sees only a status code.
async function assertFetchOk(res: Response): Promise<void> {
  if (res.ok) return;
  const body = (await res.json().catch(() => ({}))) as { error?: string; message?: string; code?: string };
  throw new ApiError(describeApiError(body), res.status);
}

export function useContainerFiles(containerId: string, path: string, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-files', envId, containerId, path],
    queryFn: () =>
      api
        .get<FileEntry[]>(`/containers/${containerId}/files`, {
          path,
          ...(envId ? { env: envId } : {}),
        })
        .then((r) => r.data ?? []),
    enabled: enabled && !!containerId,
  });
}

export function useFileContent(containerId: string, path: string, enabled = true) {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['container-file-content', envId, containerId, path],
    queryFn: async () => {
      const params = new URLSearchParams({ path });
      if (envId) params.set('env', envId);
      const res = await fetch(`/api/containers/${containerId}/files/content?${params}`, {
        credentials: 'include',
      });
      await assertFetchOk(res);
      return res.text();
    },
    enabled: enabled && !!containerId && !!path,
  });
}

export function useSaveFile(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async ({ path, content }: { path: string; content: string }) => {
      const params = new URLSearchParams({ path });
      if (envId) params.set('env', envId);
      const res = await fetch(`/api/containers/${containerId}/files/content?${params}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/octet-stream' },
        body: content,
      });
      await assertFetchOk(res);
      return res.json();
    },
    onSuccess: (_data, vars) => {
      toast.success(t('files.toast.saved'));
      void queryClient.invalidateQueries({ queryKey: ['container-file-content', envId, containerId, vars.path] });
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

export function useCreateDirectory(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async (dirPath: string) => {
      const params = new URLSearchParams({ path: dirPath });
      if (envId) params.set('env', envId);
      return api.post(`/containers/${containerId}/files/directory?${params}`).then(assertSuccess);
    },
    onSuccess: () => {
      toast.success(t('files.toast.created'));
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

export function useRenameFile(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async ({ path, newName }: { path: string; newName: string }) => {
      const params = new URLSearchParams();
      if (envId) params.set('env', envId);
      const qs = params.toString();
      return api
        .post(`/containers/${containerId}/files/rename${qs ? `?${qs}` : ''}`, { path, newName })
        .then(assertSuccess);
    },
    onSuccess: () => {
      toast.success(t('files.toast.renamed'));
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

export function useChmod(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async ({ path, mode }: { path: string; mode: string }) => {
      const params = new URLSearchParams();
      if (envId) params.set('env', envId);
      const qs = params.toString();
      return api
        .post(`/containers/${containerId}/files/chmod${qs ? `?${qs}` : ''}`, { path, mode })
        .then(assertSuccess);
    },
    onSuccess: () => {
      toast.success(t('files.toast.permissionsChanged'));
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

export function useDeleteFile(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async ({ path, recursive }: { path: string; recursive: boolean }) => {
      const params: Record<string, string> = { path, recursive: String(recursive) };
      if (envId) params.env = envId;
      return api.del(`/containers/${containerId}/files/content`, params).then(assertSuccess);
    },
    onSuccess: () => {
      toast.success(t('files.toast.deleted'));
      void queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

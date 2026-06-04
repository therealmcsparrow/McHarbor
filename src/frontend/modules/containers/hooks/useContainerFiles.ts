// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useUploadActivityStore } from '@resources/stores/upload-activity';
import type { FileEntry } from '@core/types/docker';
import { toast } from 'sonner';
import { useTranslation } from 'react-i18next';

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
      if (!res.ok) throw new Error(`Failed to fetch file: ${res.status}`);
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
      if (!res.ok) throw new Error(`Save failed: ${res.status}`);
      return res.json();
    },
    onSuccess: (_data, vars) => {
      toast.success(t('files.toast.saved'));
      queryClient.invalidateQueries({ queryKey: ['container-file-content', envId, containerId, vars.path] });
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

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

      return new Promise((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        xhr.open('POST', `/api/containers/${containerId}/files/upload?${params}`);
        xhr.withCredentials = true;
        xhr.upload.onprogress = (event) => {
          if (!event.lengthComputable) {
            setProgress(null);
            return;
          }
          setProgress(Math.min(100, Math.round((event.loaded / event.total) * 100)));
        };
        xhr.onload = () => {
          if (xhr.status < 200 || xhr.status >= 300) {
            reject(new Error(uploadErrorMessage(xhr)));
            return;
          }
          setProgress(100);
          try {
            resolve(xhr.responseText ? JSON.parse(xhr.responseText) : null);
          } catch (error) {
            reject(error);
          }
        };
        xhr.onerror = () => reject(new Error('Upload failed'));
        xhr.onabort = () => reject(new Error('Upload aborted'));
        xhr.send(form);
      });
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
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t('files.toast.uploadFailed'));
    },
    onSettled: () => {
      finishUpload(envId);
      window.setTimeout(() => setProgress(null), 600);
      window.setTimeout(() => {
        queryClient.invalidateQueries({ queryKey: ['containers'] });
        queryClient.invalidateQueries({ queryKey: ['containers-bulk-stats', envId] });
        queryClient.invalidateQueries({ queryKey: ['host-metrics', envId] });
        queryClient.invalidateQueries({ queryKey: ['docker-info', envId] });
      }, 800);
    },
  });

  return { ...mutation, progress, resetProgress: () => setProgress(null) };
}

function uploadErrorMessage(xhr: XMLHttpRequest): string {
  if (xhr.responseText) {
    try {
      const response = JSON.parse(xhr.responseText) as { error?: string; message?: string };
      return response.error || response.message || `Upload failed: ${xhr.status}`;
    } catch {
      return xhr.responseText;
    }
  }
  return `Upload failed: ${xhr.status}`;
}

export function useCreateDirectory(containerId: string) {
  const envId = useEnvironmentStore((s) => s.currentId);
  const queryClient = useQueryClient();
  const { t } = useTranslation('containers');

  return useMutation({
    mutationFn: async (dirPath: string) => {
      const params = new URLSearchParams({ path: dirPath });
      if (envId) params.set('env', envId);
      return api.post(`/containers/${containerId}/files/directory?${params}`);
    },
    onSuccess: () => {
      toast.success(t('files.toast.created'));
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
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
      return api.post(`/containers/${containerId}/files/rename${qs ? `?${qs}` : ''}`, { path, newName });
    },
    onSuccess: () => {
      toast.success(t('files.toast.renamed'));
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
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
      return api.post(`/containers/${containerId}/files/chmod${qs ? `?${qs}` : ''}`, { path, mode });
    },
    onSuccess: () => {
      toast.success(t('files.toast.permissionsChanged'));
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
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
      return api.del(`/containers/${containerId}/files/content`, params);
    },
    onSuccess: () => {
      toast.success(t('files.toast.deleted'));
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
    },
  });
}

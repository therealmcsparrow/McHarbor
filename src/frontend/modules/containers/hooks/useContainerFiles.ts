// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
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

  return useMutation({
    mutationFn: async ({ path, file }: { path: string; file: File }) => {
      const params = new URLSearchParams({ path });
      if (envId) params.set('env', envId);
      const form = new FormData();
      form.append('file', file);
      const res = await fetch(`/api/containers/${containerId}/files/upload?${params}`, {
        method: 'POST',
        credentials: 'include',
        body: form,
      });
      if (!res.ok) throw new Error(`Upload failed: ${res.status}`);
      return res.json();
    },
    onSuccess: () => {
      toast.success(t('files.toast.uploaded'));
      queryClient.invalidateQueries({ queryKey: ['container-files'] });
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

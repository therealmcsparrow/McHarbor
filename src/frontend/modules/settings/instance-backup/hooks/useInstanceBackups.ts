// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ApiResponse, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ExportStatus = 'running' | 'completed' | 'failed';
export type KeyKind = 'passphrase' | 'keyfile';

export type ExportManifest = {
  version: string;
  build: string;
  backend: string;
  createdAt: string;
  createdBy: string;
  hostname: string;
  tables: { name: string; rows: number }[];
  plugins: { slug: string; version: string; enabled: boolean }[];
  blobs: { path: string; size: number; kind: string }[];
  includesMetrics: boolean;
  key: { kind: KeyKind; keyId: string };
  note?: string;
};

export type ExportProgress = { phase: string; item?: string; done: number; total: number };

export type DestinationStatus = 'uploading' | 'success' | 'failure';

export type ExportDestination = {
  id: string;
  locationId?: string;
  locationName: string;
  locationType: string;
  status: DestinationStatus;
  remoteKey: string;
  error: string;
  uploadedAt?: string;
  bytesUploaded: number;
  bytesTotal: number;
};

export type InstanceExport = {
  id: string;
  status: ExportStatus;
  trigger: string;
  createdBy: string;
  fileName: string;
  sizeBytes: number;
  manifest?: ExportManifest;
  keyKind: KeyKind;
  keyId: string;
  includeMetrics: boolean;
  note: string;
  error: string;
  verifyStatus: string;
  verifyError: string;
  verifiedAt?: string;
  completedAt?: string;
  createdAt: string;
  progress?: ExportProgress;
  destinations?: ExportDestination[];
};

export type CreateExportInput = {
  passphrase?: string;
  keyFile?: string;
  includeMetrics: boolean;
  note?: string;
  /** Copy the finished export to each of these storage locations; omit to keep it here. */
  locationIds?: string[];
};

export class InstanceBackupRefusal extends Error {
  constructor(
    readonly reason: string,
    readonly detail = '',
  ) {
    super(detail || reason);
  }
}

export function refusalOf<T>(res: ApiResponse<T>): T {
  if (!res.success) {
    const data = res.data as { reason?: unknown; detail?: unknown } | undefined;
    if (typeof data?.reason === 'string') {
      throw new InstanceBackupRefusal(data.reason, typeof data.detail === 'string' ? data.detail : '');
    }
  }
  return assertSuccess(res);
}

const KEY = ['instance-backups'] as const;

export function useInstanceBackups(enabled = true) {
  return useQuery({
    queryKey: KEY,
    enabled,
    queryFn: () =>
      api.get<PaginatedData<InstanceExport>>('/instance-backups', { per_page: '100' }).then((r) => r.data?.items ?? []),
    refetchInterval: (query) =>
      (query.state.data ?? []).some((e) => e.status === 'running' || e.verifyStatus === 'running') ? 2_000 : 30_000,
  });
}

export function useGenerateKeyFile() {
  return useMutation({
    mutationFn: () => api.post<{ keyFile: string; fileName: string }>('/instance-backups/key-file').then(assertSuccess),
  });
}

export function useCreateExport() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateExportInput) => api.post<InstanceExport>('/instance-backups', input).then(refusalOf),
    onSuccess: () => client.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDeleteExport() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/instance-backups/${id}`).then(assertSuccess),
    onSuccess: () => client.invalidateQueries({ queryKey: KEY }),
  });
}

export function useVerifyExport() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...key }: { id: string; passphrase?: string; keyFile?: string }) =>
      api.post<InstanceExport>(`/instance-backups/${encodeURIComponent(id)}/verify`, key).then(refusalOf),
    onSuccess: () => client.invalidateQueries({ queryKey: KEY }),
  });
}

export function useDownloadStepUp() {
  return useMutation({
    mutationFn: ({ password, totpCode }: { password?: string; totpCode?: string }) =>
      api
        .post<{ expiresAt: string }>('/auth/sudo', {
          scope: 'instance_backup.download',
          password,
          totpCode,
        })
        .then(assertSuccess),
  });
}

export function downloadURL(id: string): string {
  return `/api/instance-backups/${encodeURIComponent(id)}/download`;
}

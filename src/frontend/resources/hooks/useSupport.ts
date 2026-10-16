// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess, describeApiError } from '@resources/utils/api-mutation';

export type CheckStatus = 'pass' | 'warn' | 'fail' | 'skip';

export type CheckResult = {
  id: string;
  status: CheckStatus;
  reason: string;
  details?: Record<string, string>;
  durationMs: number;
};

export type DiagnosticsReport = {
  generatedAt: string;
  version: string;
  build: string;
  checks: CheckResult[];
};

export type KeySource = { source: 'env' | 'file'; fileSeen: boolean };

export type InventoryEntry = {
  category: string;
  path: string;
  kind: 'file' | 'dir' | 'missing' | 'external';
  exists: boolean;
  sizeBytes: number;
  files: number;
  truncated?: boolean;
  writable: boolean;
  backup: 'included' | 'partial' | 'excluded' | 'database';
  cleanup: 'no' | 'safe' | 'managed' | 'regenerable' | 'unknown';
};

export type Inventory = {
  dataDir: string;
  databaseDriver: string;
  totalBytes: number;
  freeBytes: number;
  diskBytes: number;
  entries: InventoryEntry[];
};

export const BUNDLE_CATEGORIES = [
  'system',
  'logs',
  'plugins',
  'cluster',
  'database',
  'environments',
  'audit',
  'diagnostics',
] as const;
export type BundleCategory = (typeof BUNDLE_CATEGORIES)[number];

export type Bundle = {
  id: string;
  status: 'running' | 'ready' | 'failed';
  categories: BundleCategory[];
  createdAt: string;
  createdBy: string;
  finishedAt?: string;
  sizeBytes: number;
  progress: number;
  step?: string;
  error?: string;
};

export type BundleList = { items: Bundle[]; keep: number };

export type PreviewFile = { name: string; sizeBytes: number; content: string; truncated?: boolean };
export type BundlePreview = { categories: BundleCategory[]; totalBytes: number; files: PreviewFile[] };

export type PluginCompat = { slug: string; version: string; range: string; compatible: 'yes' | 'no' | 'unknown' };

export type PreUpgradeReport = {
  currentVersion: string;
  targetVersion: string;
  checks: CheckResult[];
  plugins: PluginCompat[];
  databaseDriver: string;
};

const keys = {
  diagnostics: ['support', 'diagnostics'] as const,
  inventory: ['support', 'inventory'] as const,
  keySource: ['support', 'encryption-key'] as const,
  bundles: ['support', 'bundles'] as const,
};

export function useDiagnostics(enabled: boolean) {
  return useQuery({
    queryKey: keys.diagnostics,
    enabled,
    queryFn: () => api.get<DiagnosticsReport>('/support/diagnostics').then(assertSuccess),
    staleTime: Infinity,
  });
}

export function useRunCheck() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.get<DiagnosticsReport>('/support/diagnostics', { check: id }).then(assertSuccess),
    onSuccess: (single) => {
      client.setQueryData<DiagnosticsReport>(keys.diagnostics, (prev) => {
        if (!prev) return single;
        const byId = new Map(single.checks.map((c) => [c.id, c]));
        return { ...prev, generatedAt: single.generatedAt, checks: prev.checks.map((c) => byId.get(c.id) ?? c) };
      });
    },
  });
}

export function useStateInventory(enabled: boolean) {
  return useQuery({
    queryKey: keys.inventory,
    enabled,
    queryFn: () => api.get<Inventory>('/support/inventory').then(assertSuccess),
  });
}

export function useEncryptionKeySource(enabled: boolean) {
  return useQuery({
    queryKey: keys.keySource,
    enabled,
    queryFn: () => api.get<KeySource>('/support/encryption-key').then(assertSuccess),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

export function useSupportBundles(enabled: boolean) {
  return useQuery({
    queryKey: keys.bundles,
    enabled,
    queryFn: () => api.get<BundleList>('/support/bundles').then(assertSuccess),
    refetchInterval: (query) => (query.state.data?.items.some((b) => b.status === 'running') ? 1_000 : false),
  });
}

export function useBundlePreview() {
  return useMutation({
    mutationFn: (categories: BundleCategory[]) =>
      api.get<BundlePreview>('/support/bundles/preview', { categories: categories.join(',') }).then(assertSuccess),
  });
}

export function useGenerateBundle() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (categories: BundleCategory[]) =>
      api.post<Bundle>('/support/bundles', { categories }).then(assertSuccess),
    onSettled: () => void client.invalidateQueries({ queryKey: keys.bundles }),
  });
}

export function useDeleteBundle() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.del<{ id: string }>(`/support/bundles/${encodeURIComponent(id)}`).then(assertSuccess),
    onSettled: () => void client.invalidateQueries({ queryKey: keys.bundles }),
  });
}

export function useDownloadBundle() {
  return useMutation({
    mutationFn: async (id: string) => {
      const res = await api.download(`/support/bundles/${encodeURIComponent(id)}/download`);
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as Parameters<typeof describeApiError>[0];
        throw new Error(describeApiError(body));
      }
      const url = URL.createObjectURL(await res.blob());
      const a = document.createElement('a');
      a.href = url;
      a.download = `mcharbor-support-${id}.zip`;
      a.click();
      URL.revokeObjectURL(url);
    },
  });
}

export function usePreUpgradeCheck(target: string, enabled: boolean) {
  return useQuery({
    queryKey: ['support', 'pre-upgrade', target],
    enabled,
    queryFn: () => api.get<PreUpgradeReport>('/support/pre-upgrade', { target }).then(assertSuccess),
    staleTime: 0,
  });
}

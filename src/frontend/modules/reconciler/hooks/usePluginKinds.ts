// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';

export type KindOption = { value: string; label: string; detail?: string };

export type KindField = {
  key: string;
  label: string;
  description?: string;
  type: 'select' | 'text' | 'number' | 'bool' | (string & {});
  options?: KindOption[];
  dynamic?: boolean;
  unit?: string;
  min?: number;
  max?: number;
};

export type PluginKind = {
  source: string;
  kind: string;
  label: string;
  description?: string;
  subjectLabel: string;
  enforceable: boolean;
  fields: KindField[];
  ownScopes: boolean;
};

const BUILT_IN = ['container', 'stack', 'environment'];

export function usePluginKinds() {
  const allowed = usePermission(PERM.RECONCILER_VIEW);
  return useQuery({
    enabled: allowed,
    queryKey: ['reconciler', 'kinds'],
    queryFn: () => api.get<PluginKind[]>('/reconciler/kinds').then((r) => r.data ?? []),
    staleTime: 60_000,
  });
}

function kindPath(k: Pick<PluginKind, 'source' | 'kind'>) {
  return `/reconciler/kinds/${encodeURIComponent(k.source)}/${encodeURIComponent(k.kind)}`;
}

export function usePluginScopes(kind: PluginKind | undefined) {
  return useQuery({
    enabled: !!kind?.ownScopes,
    queryKey: ['reconciler', 'kinds', kind?.source, kind?.kind, 'scopes'],
    queryFn: () => api.get<KindOption[]>(`${kindPath(kind as PluginKind)}/scopes`).then((r) => r.data ?? []),
  });
}

export function usePluginSubjects(kind: PluginKind | undefined, env: string) {
  return useQuery({
    enabled: !!kind && env !== '',
    queryKey: ['reconciler', 'kinds', kind?.source, kind?.kind, 'subjects', env],
    queryFn: () => api.get<KindOption[]>(`${kindPath(kind as PluginKind)}/subjects`, { env }).then((r) => r.data ?? []),
  });
}

export function usePluginFieldOptions(kind: PluginKind | undefined, field: KindField, env: string) {
  return useQuery({
    enabled: !!kind && !!field.dynamic && env !== '',
    queryKey: ['reconciler', 'kinds', kind?.source, kind?.kind, 'options', field.key, env],
    queryFn: () =>
      api
        .get<KindOption[]>(`${kindPath(kind as PluginKind)}/options`, { env, field: field.key })
        .then((r) => r.data ?? []),
  });
}

export async function fetchPluginCurrent(
  kind: PluginKind,
  env: string,
  subject: string,
): Promise<Record<string, string>> {
  const r = await api.get<Record<string, string>>(`${kindPath(kind)}/current`, { env, subject });
  if (!r.success) throw new Error(r.error ?? 'current');
  return r.data ?? {};
}

export function useKindLabel() {
  const { t } = useTranslation('reconciler');
  const kinds = usePluginKinds();
  const byKind = useMemo(() => new Map((kinds.data ?? []).map((k) => [k.kind, k.label])), [kinds.data]);
  return useCallback(
    (kind: string) => (BUILT_IN.includes(kind) ? t(`kind.${kind}`) : (byKind.get(kind) ?? kind)),
    [byKind, t],
  );
}

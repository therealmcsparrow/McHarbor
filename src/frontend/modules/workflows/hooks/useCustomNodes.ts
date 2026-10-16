// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { CustomNodeWithCode } from './useCustomNodeSync';

/**
 * The authoring half of custom nodes.
 *
 * `useCustomNodeSync` has always read `GET /custom-nodes` and merged the result
 * into the palette, so `custom_nodes.view` was reachable. The four mutating
 * routes were enforced by `custom_nodes.manage`, which no screen reached —
 * these hooks are what closes that.
 *
 * The query key matches `useCustomNodeSync`'s exactly, so a create or edit
 * refreshes the palette on the open canvas without a reload.
 */
export const CUSTOM_NODES_KEY = ['custom-nodes'] as const;

/** What the editor sends. The backend splits it into definition + code + i18n. */
export type CustomNodeInput = {
  definition: Omit<CustomNodeWithCode, 'code' | 'translations'>;
  code: string;
  translations: Record<string, Record<string, unknown>>;
};

export type CustomNodeTestInput = {
  code: string;
  config: Record<string, unknown>;
  msg: Record<string, unknown>;
};

export type CustomNodeTestResult = {
  port: string;
  msg: Record<string, unknown>;
  logs: string[];
  error?: string;
  duration: number;
};

export function useCreateCustomNode() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: (input: CustomNodeInput) => api.post<CustomNodeWithCode>('/custom-nodes', input).then(assertSuccess),
    meta: {
      success: t('customNodes.toast.created'),
      error: t('customNodes.toast.createFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CUSTOM_NODES_KEY });
    },
  });
}

export function useUpdateCustomNode() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: ({ key, input }: { key: string; input: CustomNodeInput }) =>
      api.put<CustomNodeWithCode>(`/custom-nodes/${encodeURIComponent(key)}`, input).then(assertSuccess),
    meta: {
      success: t('customNodes.toast.updated'),
      error: t('customNodes.toast.updateFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CUSTOM_NODES_KEY });
    },
  });
}

export function useDeleteCustomNode() {
  const queryClient = useQueryClient();
  const { t } = useTranslation('common');
  return useMutation({
    mutationFn: (key: string) => api.del(`/custom-nodes/${encodeURIComponent(key)}`).then(assertSuccess),
    meta: {
      success: t('customNodes.toast.deleted'),
      error: t('customNodes.toast.deleteFailed'),
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: CUSTOM_NODES_KEY });
    },
  });
}

/**
 * Dry-runs a script in the goja sandbox without saving it.
 *
 * No success toast: the result panel *is* the feedback, and a toast on every
 * test run would be noise while iterating on a script.
 */
export function useTestCustomNode() {
  return useMutation({
    mutationFn: (input: CustomNodeTestInput) =>
      api.post<CustomNodeTestResult>('/custom-nodes/test', input).then(assertSuccess),
  });
}

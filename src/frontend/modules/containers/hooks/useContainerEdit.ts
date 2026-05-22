// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useRef, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ContainerInspect } from '@core/types/docker';
import {
  type EditFormData,
  type ChangeClassification,
  containerToEditForm,
  classifyChanges,
  buildUpdatePayload,
  buildRecreatePayload,
} from '../types/edit-form';

export function useContainerEdit(container: ContainerInspect | undefined) {
  const { t } = useTranslation('containers');
  const queryClient = useQueryClient();
  const envId = useEnvironmentStore((s) => s.currentId);
  const [editing, setEditing] = useState(false);
  const [editData, setEditData] = useState<EditFormData | null>(null);
  const originalRef = useRef<EditFormData | null>(null);

  const startEditing = useCallback(() => {
    if (!container) return;
    const form = containerToEditForm(container);
    originalRef.current = form;
    setEditData({
      ...form,
      env: [...form.env],
      labels: { ...form.labels },
      gpuDeviceIds: [...form.gpuDeviceIds],
      gpuCapabilities: [...form.gpuCapabilities],
    });
    setEditing(true);
  }, [container]);

  const cancelEditing = useCallback(() => {
    setEditing(false);
    setEditData(null);
    originalRef.current = null;
  }, []);

  const onFieldChange = useCallback(<K extends keyof EditFormData>(
    field: K,
    value: EditFormData[K],
  ) => {
    setEditData((prev) => (prev ? { ...prev, [field]: value } : prev));
  }, []);

  const changes: ChangeClassification = useMemo(() => {
    if (!originalRef.current || !editData) {
      return { hasResourceChanges: false, hasConfigChanges: false, changedResourceFields: [], changedConfigFields: [] };
    }
    return classifyChanges(originalRef.current, editData);
  }, [editData]);

  const envQuery = envId ? `?env=${envId}` : '';

  const updateMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      api.post(`/containers/${container?.Id}/update${envQuery}`, payload).then(assertSuccess),
    meta: { success: t('toast.updated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container'] });
      queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });

  const recreateMutation = useMutation({
    mutationFn: (payload: Record<string, unknown>) =>
      api.post(`/containers/${container?.Id}/recreate${envQuery}`, payload).then(assertSuccess),
    meta: { success: t('toast.recreated') },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['container'] });
      queryClient.invalidateQueries({ queryKey: ['containers'] });
    },
  });

  const save = useCallback(async () => {
    if (!editData || !container) return;

    if (changes.hasResourceChanges && !changes.hasConfigChanges) {
      const payload = buildUpdatePayload(editData, changes);
      await updateMutation.mutateAsync(payload);
    } else if (changes.hasConfigChanges) {
      // If both resource and config changes, do update first then recreate
      if (changes.hasResourceChanges) {
        const updatePayload = buildUpdatePayload(editData, changes);
        await updateMutation.mutateAsync(updatePayload);
      }
      const recreatePayload = buildRecreatePayload(editData, changes);
      await recreateMutation.mutateAsync(recreatePayload);
    }

    cancelEditing();
  }, [editData, container, changes, updateMutation, recreateMutation, cancelEditing]);

  const isSaving = updateMutation.isPending || recreateMutation.isPending;

  return {
    editing,
    editData,
    startEditing,
    cancelEditing,
    onFieldChange,
    save,
    changes,
    isSaving,
  };
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router';
import { useEnvironmentStore } from '@resources/stores/environment';
import { runWithMcHarborRestart } from '@resources/stores/mcharbor-restart';
import type { ContainerInspect } from '@core/types/docker';
import {
  type EditFormData,
  type ChangeClassification,
  type ResourceEditField,
  containerToEditForm,
  classifyChanges,
  buildUpdatePayload,
  buildRecreatePayload,
} from '../types/edit-form';

import { useContainerEditMutations } from './useContainerEditMutations';

type ContainerEditOptions = {
  unlockProtected?: boolean;
};

export function useContainerEdit(container: ContainerInspect | undefined, options: ContainerEditOptions = {}) {
  const routerLocation = useLocation();
  const navigate = useNavigate();
  const envId = useEnvironmentStore((s) => s.currentId);
  const [editing, setEditing] = useState(false);
  const [editData, setEditData] = useState<EditFormData | null>(null);
  const [originalForm, setOriginal] = useState<EditFormData | null>(null);

  const startEditing = useCallback(() => {
    if (!container) return;
    const form = containerToEditForm(container);
    setOriginal(structuredClone(form));
    setEditData(structuredClone(form));
    setEditing(true);
  }, [container]);

  const cancelEditing = useCallback(() => {
    setEditing(false);
    setEditData(null);
    setOriginal(null);
  }, []);

  const onFieldChange = useCallback(<K extends keyof EditFormData>(field: K, value: EditFormData[K]) => {
    setEditData((prev) => (prev ? { ...prev, [field]: value } : prev));
  }, []);

  const changes: ChangeClassification = useMemo(() => {
    if (!originalForm || !editData) {
      return {
        hasResourceChanges: false,
        hasConfigChanges: false,
        changedResourceFields: [],
        changedConfigFields: [],
      };
    }
    return classifyChanges(originalForm, editData);
  }, [originalForm, editData]);

  const { updateMutation, recreateMutation } = useContainerEditMutations(container, envId);

  const save = useCallback(async () => {
    if (!editData || !container) return;

    if (changes.hasResourceChanges && !changes.hasConfigChanges) {
      const payload = buildUpdatePayload(editData, changes);
      if (options.unlockProtected) payload.unlockProtected = true;
      await updateMutation.mutateAsync(payload);
    } else if (changes.hasConfigChanges) {
      const recreatePayload = buildRecreatePayload(editData, changes);
      if (options.unlockProtected) recreatePayload.unlockProtected = true;
      // The McHarbor container recreates itself through a detached helper, so
      // the API disappears from under this page moments after the request
      // returns. Driving the global restart dialog is what keeps the page
      // honest — and it reloads once the new instance answers, instead of
      // leaving a success toast on a dead session. unlockProtected is only ever
      // true for that container, so it doubles as the self-container signal.
      const recreated = options.unlockProtected
        ? await runWithMcHarborRestart(() => recreateMutation.mutateAsync(recreatePayload))
        : await recreateMutation.mutateAsync(recreatePayload);
      cancelEditing();
      if (recreated?.confirmed && recreated.Id && recreated.Id !== container.Id) {
        void navigate(`/containers/${encodeURIComponent(recreated.Id)}${routerLocation.search}`, {
          replace: true,
        });
      }
      return;
    }

    cancelEditing();
  }, [
    editData,
    container,
    changes,
    updateMutation,
    recreateMutation,
    cancelEditing,
    options.unlockProtected,
    routerLocation.search,
    navigate,
  ]);

  // Recreate on demand, with no configuration change: an empty payload keeps
  // the current image and every current setting, which is exactly what the
  // detail hero's Recreate button promises. It shares the edit flow's
  // self-restart handling and its follow-the-new-id navigation, because the
  // container id changes under the page either way.
  const recreateInPlace = useCallback(
    async (recreateOptions: { secretFileMode?: boolean } = {}) => {
      if (!container) return;
      const payload: Record<string, unknown> = {};
      if (options.unlockProtected) payload.unlockProtected = true;
      if (recreateOptions.secretFileMode !== undefined) {
        payload.secretFileMode = recreateOptions.secretFileMode;
      }
      const recreated = options.unlockProtected
        ? await runWithMcHarborRestart(() => recreateMutation.mutateAsync(payload))
        : await recreateMutation.mutateAsync(payload);
      cancelEditing();
      if (recreated?.confirmed && recreated.Id && recreated.Id !== container.Id) {
        void navigate(`/containers/${encodeURIComponent(recreated.Id)}${routerLocation.search}`, { replace: true });
      }
    },
    [cancelEditing, container, routerLocation.search, navigate, options.unlockProtected, recreateMutation],
  );

  const saveResourceField = useCallback(
    async (field: ResourceEditField, value: number | string) => {
      if (!container) return;
      const original = containerToEditForm(container);
      const current = { ...original, [field]: value };
      const fieldChanges = classifyChanges(original, current);
      if (!fieldChanges.hasResourceChanges) return;
      const payload = buildUpdatePayload(current, fieldChanges);
      if (options.unlockProtected) payload.unlockProtected = true;
      await updateMutation.mutateAsync(payload);
    },
    [container, options.unlockProtected, updateMutation],
  );

  const isSaving = updateMutation.isPending || recreateMutation.isPending;

  return {
    editing,
    editData,
    startEditing,
    cancelEditing,
    onFieldChange,
    save,
    recreateInPlace,
    saveResourceField,
    changes,
    isSaving,
    recreateError: recreateMutation.error?.message ?? null,
    clearRecreateError: recreateMutation.reset,
  };
}

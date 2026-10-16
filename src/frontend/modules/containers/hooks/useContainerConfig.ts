// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import type { ContainerInspect } from '@core/types/docker';
import type { EditFormData } from '../types/edit-form';
import { useContainerEdit } from './useContainerEdit';

type ContainerConfigOptions = {
  unlockProtected?: boolean;
  /** Optional callback fired after a successful save. */
  onSaved?: () => void;
};

/**
 * useContainerConfig is a thin, settings-shell-friendly facade over
 * useContainerEdit. It exposes the same start/cancel/save lifecycle
 * but returns the shape a SettingsShell-style form expects: a single
 * `dirty` flag, a `save` handler, and a `cancel` handler — without
 * exposing the per-field mutation API the edit tabs use.
 *
 * Page-level edit forms (config-edit dialogs, the SettingsShell in
 * ContainerDetailPage) prefer this hook. The per-tab forms keep using
 * `useContainerEdit` directly so they can write individual fields.
 */
export function useContainerConfig(container: ContainerInspect | undefined, options: ContainerConfigOptions = {}) {
  const { t } = useTranslation('containers');
  const edit = useContainerEdit(container, {
    unlockProtected: options.unlockProtected,
  });

  const dirty = useMemo(() => {
    if (!edit.editData) return false;
    return edit.changes.hasResourceChanges || edit.changes.hasConfigChanges;
  }, [edit.editData, edit.changes]);

  const start = useCallback(() => {
    edit.startEditing();
  }, [edit]);

  const cancel = useCallback(() => {
    edit.cancelEditing();
  }, [edit]);

  const save = useCallback(async () => {
    if (!edit.editData) return;
    try {
      await edit.save();
      toast.success(t('toast.updated'));
      options.onSaved?.();
    } catch {
      // The edit mutations report their own failure through the global mutation cache.
    }
  }, [edit, options, t]);

  const setField = useCallback(
    <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => {
      edit.onFieldChange(field, value);
    },
    [edit],
  );

  return {
    editing: edit.editing,
    data: edit.editData,
    dirty,
    isSaving: edit.isSaving,
    changes: edit.changes,
    start,
    cancel,
    save,
    setField,
  };
}

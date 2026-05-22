// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import {
  createSearchPresetId,
  loadSearchPresets,
  persistSearchPresets,
  type SearchPreset,
} from '@resources/utils/search-filter';

export function useSavedSearchFilters<TState>(storageKey: string) {
  const [presets, setPresets] = useState<SearchPreset<TState>[]>([]);
  const [selectedPresetId, setSelectedPresetId] = useState('');

  useEffect(() => {
    setPresets(loadSearchPresets<TState>(storageKey));
    setSelectedPresetId('');
  }, [storageKey]);

  useEffect(() => {
    persistSearchPresets(storageKey, presets);
  }, [presets, storageKey]);

  const selectedPreset = presets.find((preset) => preset.id === selectedPresetId) ?? null;

  const savePreset = (label: string, state: TState) => {
    const trimmed = label.trim();
    if (!trimmed) {
      return;
    }

    const nextPreset: SearchPreset<TState> = {
      id: createSearchPresetId(),
      label: trimmed,
      state,
    };

    setPresets((current) => [nextPreset, ...current.filter((preset) => preset.label !== trimmed)].slice(0, 12));
    setSelectedPresetId(nextPreset.id);
  };

  const deleteSelectedPreset = () => {
    if (!selectedPresetId) {
      return;
    }

    setPresets((current) => current.filter((preset) => preset.id !== selectedPresetId));
    setSelectedPresetId('');
  };

  return {
    presets,
    selectedPresetId,
    setSelectedPresetId,
    selectedPreset,
    savePreset,
    deleteSelectedPreset,
  };
}

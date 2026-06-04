// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';

type UploadActivityState = {
  activeByEnv: Record<string, number>;
  startUpload: (envId: string) => void;
  finishUpload: (envId: string) => void;
};

export function uploadActivityKey(envId: string) {
  return envId || 'default';
}

export const useUploadActivityStore = create<UploadActivityState>()((set) => ({
  activeByEnv: {},
  startUpload: (envId) =>
    set((state) => {
      const key = uploadActivityKey(envId);
      return { activeByEnv: { ...state.activeByEnv, [key]: (state.activeByEnv[key] ?? 0) + 1 } };
    }),
  finishUpload: (envId) =>
    set((state) => {
      const key = uploadActivityKey(envId);
      const next = { ...state.activeByEnv };
      const count = (next[key] ?? 0) - 1;
      if (count > 0) {
        next[key] = count;
      } else {
        delete next[key];
      }
      return { activeByEnv: next };
    }),
}));

export function useEnvironmentUploadActive(envId: string) {
  return useUploadActivityStore((state) => (state.activeByEnv[uploadActivityKey(envId)] ?? 0) > 0);
}

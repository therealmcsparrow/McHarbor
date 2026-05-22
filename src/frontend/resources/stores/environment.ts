// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

type Environment = {
  id: string;
  name: string;
  orchestratorType: 'docker' | 'kubernetes';
  connectionType: string;
  isDefault: boolean;
  dockerVersion: string | null;
  k8sVersion: string | null;
  scheduledUpdateCheckEnabled: boolean;
  automaticImagePruningEnabled: boolean;
  trackContainerEventsEnabled: boolean;
  collectContainerMetricsEnabled: boolean;
  highlightContainerChangesEnabled: boolean;
  dockerDiskUsageNotificationsEnabled: boolean;
  dockerDiskUsageThresholdPercent: number;
};

type EnvironmentState = {
  currentId: string;
  environments: Environment[];
  setCurrentId: (id: string) => void;
  setEnvironments: (envs: Environment[]) => void;
  upsertEnvironment: (env: Environment) => void;
};

export const useEnvironmentStore = create<EnvironmentState>()(
  persist(
    (set) => ({
      currentId: '',
      environments: [],
      setCurrentId: (id) => set({ currentId: id }),
      setEnvironments: (environments) => set({ environments }),
      upsertEnvironment: (environment) =>
        set((state) => {
          const index = state.environments.findIndex((item) => item.id === environment.id);
          if (index === -1) {
            return { environments: [...state.environments, environment] };
          }

          const environments = [...state.environments];
          environments[index] = { ...environments[index], ...environment };
          return { environments };
        }),
    }),
    { name: 'mcharbor-environment' }
  )
);

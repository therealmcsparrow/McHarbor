// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type WizardStepId =
  | 'welcome'
  | 'environment'
  | 'app'
  | 'dashboard'
  | 'notifications'
  | 'finish';

export type WizardStep = {
  id: WizardStepId;
  titleKey: string;
  descriptionKey: string;
  optional?: boolean;
};

export const WIZARD_STEPS: WizardStep[] = [
  { id: 'welcome', titleKey: 'wizard.welcome.title', descriptionKey: 'wizard.welcome.description' },
  { id: 'environment', titleKey: 'wizard.environment.title', descriptionKey: 'wizard.environment.description' },
  { id: 'app', titleKey: 'wizard.app.title', descriptionKey: 'wizard.app.description' },
  { id: 'dashboard', titleKey: 'wizard.dashboard.title', descriptionKey: 'wizard.dashboard.description' },
  { id: 'notifications', titleKey: 'wizard.notifications.title', descriptionKey: 'wizard.notifications.description', optional: true },
  { id: 'finish', titleKey: 'wizard.finish.title', descriptionKey: 'wizard.finish.description' },
];

type OnboardingState = {
  hasCompletedWizard: boolean;
  hasSkippedWizard: boolean;
  hasSeenTour: boolean;
  completedSteps: WizardStepId[];
  dismissedHints: string[];
  setStepCompleted: (id: WizardStepId) => void;
  setHasCompletedWizard: (value: boolean) => void;
  setHasSkippedWizard: (value: boolean) => void;
  setHasSeenTour: (value: boolean) => void;
  dismissHint: (id: string) => void;
  reset: () => void;
};

const STORAGE_KEY = 'mcharbor-onboarding-v1';

export const useOnboardingStore = create<OnboardingState>()(
  persist(
    (set) => ({
      hasCompletedWizard: false,
      hasSkippedWizard: false,
      hasSeenTour: false,
      completedSteps: [],
      dismissedHints: [],

      setStepCompleted: (id) =>
        set((s) => ({
          completedSteps: s.completedSteps.includes(id)
            ? s.completedSteps
            : [...s.completedSteps, id],
        })),

      setHasCompletedWizard: (value) => set({ hasCompletedWizard: value }),
      setHasSkippedWizard: (value) => set({ hasSkippedWizard: value }),
      setHasSeenTour: (value) => set({ hasSeenTour: value }),

      dismissHint: (id) =>
        set((s) => ({
          dismissedHints: s.dismissedHints.includes(id)
            ? s.dismissedHints
            : [...s.dismissedHints, id],
        })),

      reset: () =>
        set({
          hasCompletedWizard: false,
          hasSkippedWizard: false,
          hasSeenTour: false,
          completedSteps: [],
          dismissedHints: [],
        }),
    }),
    {
      name: STORAGE_KEY,
      version: 1,
    },
  ),
);

export function isFirstRun(state: OnboardingState): boolean {
  return !state.hasCompletedWizard && !state.hasSkippedWizard;
}

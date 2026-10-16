// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { ErrorDetails } from '@resources/utils/api-mutation';

type ErrorDetailsState = {
  open: ErrorDetails | null;
  show: (details: ErrorDetails) => void;
  close: () => void;
};

/**
 * The failure whose details dialog is open. A toast action has no React tree of
 * its own to render a dialog in, so it hands the details to this store and the
 * one dialog mounted beside the toaster renders them.
 */
export const useErrorDetails = create<ErrorDetailsState>((set) => ({
  open: null,
  show: (details) => set({ open: details }),
  close: () => set({ open: null }),
}));

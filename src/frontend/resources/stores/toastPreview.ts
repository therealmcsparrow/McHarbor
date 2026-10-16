// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';

type ToastPreviewState = {
  /**
   * `visibleToasts` to apply while a preview is running, or null when none is.
   */
  maxVisible: number | null;
  begin: (maxVisible: number) => void;
  end: () => void;
};

/**
 * A temporary `visibleToasts` override for the notification settings preview.
 *
 * Everything else the preview needs — position, rich colors, close button,
 * duration — sonner accepts per toast, so the draft values can be handed
 * straight to `toast()` and no shared state is involved. Stacking is the one
 * exception: `visibleToasts` is a `<Toaster>` prop with no per-toast
 * equivalent, so previewing "show 6 at once" while the saved value is 3 needs
 * the mounted toaster to be told, briefly, to allow 6.
 *
 * The store is deliberately narrow. It does not hold the draft settings, is
 * never persisted, and is cleared as soon as the preview toasts are gone — a
 * preview must not be able to leave the live toaster in a state nobody chose.
 */
export const useToastPreview = create<ToastPreviewState>((set) => ({
  maxVisible: null,
  begin: (maxVisible) => set({ maxVisible }),
  end: () => set({ maxVisible: null }),
}));

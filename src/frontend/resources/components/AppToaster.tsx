// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Toaster } from 'sonner';
import { ErrorDetailsDialog } from '@resources/components/ErrorDetailsDialog';
import { useResolvedToastSettings } from '@resources/hooks/useResolvedToastSettings';
import { useToastPreview } from '@resources/stores/toastPreview';

/**
 * The one toaster the app mounts, driven by the resolved toast settings
 * rather than by literals.
 *
 * This exists because `main.tsx` cannot call hooks: it renders the provider
 * tree, so anything that needs the auth store or a query has to be a
 * component inside it. Keeping the element here also means the fallback
 * values live in one place — `useResolvedToastSettings` — instead of being
 * duplicated between the render and the settings form.
 *
 * `theme="system"` is not configurable and deliberately so: the toast follows
 * the app's own light/dark state, and a toast that ignored it would be the
 * one surface on screen in the wrong palette.
 */
export function AppToaster() {
  const { position, durationMs, maxVisible, richColors, closeButton } = useResolvedToastSettings();
  // Only ever set while the notification settings preview is on screen, and
  // cleared the moment it is gone. Every other knob the preview varies is
  // passed per toast and never touches this component.
  const previewMaxVisible = useToastPreview((s) => s.maxVisible);

  return (
    <>
      <Toaster
        position={position}
        theme="system"
        richColors={richColors}
        // A duration of 0 means "never auto-dismiss". sonner reads 0 as
        // "use the default", so the intent has to be expressed as Infinity.
        // The backend refuses 0 without a close button, so a toast reaching
        // this branch always has a way to be dismissed.
        duration={durationMs === 0 ? Infinity : durationMs}
        visibleToasts={previewMaxVisible ?? maxVisible}
        closeButton={durationMs === 0 ? true : closeButton}
      />
      <ErrorDetailsDialog />
    </>
  );
}

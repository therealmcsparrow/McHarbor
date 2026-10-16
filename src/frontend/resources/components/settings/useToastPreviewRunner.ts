// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { appToast } from '@resources/utils/toast';
import { useToastPreview } from '@resources/stores/toastPreview';
import type { ToastSettings } from '@resources/types/settings';

/** The five display knobs, without the operator-only allowUserOverride flag. */
export type ToastKnobs = Omit<ToastSettings, 'allowUserOverride'>;

// A preview that never auto-dismisses still has to release the stacking
// override eventually, or the live toaster keeps the previewed height. Long
// enough to read and dismiss by hand, short enough not to linger.
const PREVIEW_HOLD_MS = 20000;

// Samples arrive one at a time rather than as a block, which is how real
// notifications turn up. Note the trade-off: with a duration shorter than
// (maxVisible - 1) x this interval, the early samples dismiss before the last
// one lands, so the full stack height is not visible. Spacing them out is
// worth more than demonstrating the ceiling.
const PREVIEW_INTERVAL_MS = 2000;

/**
 * Raises sample toasts built entirely from the values in the form, so the
 * preview answers "what am I about to save" rather than "what is live".
 *
 * sonner takes position, rich colors, close button and duration per toast,
 * so those four need nothing but the draft. Stacking is the exception —
 * `visibleToasts` is a property of the toaster, not of a toast — so the
 * override goes through the preview store and is released on a timer.
 *
 * The samples are spaced out rather than raised together, so the preview
 * shows notifications arriving rather than a block appearing.
 */
export function useToastPreviewRunner(value: ToastKnobs): () => void {
  const { t } = useTranslation('settings');
  const beginPreview = useToastPreview((s) => s.begin);
  const endPreview = useToastPreview((s) => s.end);
  // One entry per scheduled sample plus the one that releases the override.
  // They are held together because every exit path — a second press, leaving
  // the page — has to cancel all of them, not just the last.
  const timers = useRef<number[]>([]);
  const neverDismisses = value.durationMs === 0;

  function clearTimers() {
    timers.current.forEach((id) => window.clearTimeout(id));
    timers.current = [];
  }

  // Leaving the page mid-preview must not strand the override on the live
  // toaster: whoever navigates away would keep the previewed stack height
  // until a reload, and any sample still queued would appear on whatever page
  // they landed on.
  useEffect(() => {
    return () => {
      timers.current.forEach((id) => window.clearTimeout(id));
      timers.current = [];
      endPreview();
    };
  }, [endPreview]);

  return function handlePreview() {
    clearTimers();

    // Clear whatever is on screen first. Without this, a second press stacks
    // against the toasts from the first and the count reads wrong.
    appToast.dismiss();

    // Rich colors are invisible on a single success toast — the whole point
    // of the setting is that severities look different — so the preview
    // cycles three. Chosen by branch rather than by indexing an array: the
    // t() keys stay literal, which is what the i18n CI gate reads, and there
    // is no index for the type checker to call possibly-undefined.
    function sampleFor(index: number) {
      switch (index % 3) {
        case 0:
          return { raise: appToast.success, title: t('notifications.previewTitle.success') };
        case 1:
          return { raise: appToast.warning, title: t('notifications.previewTitle.warning') };
        default:
          return { raise: appToast.error, title: t('notifications.previewTitle.error') };
      }
    }

    const count = value.maxVisible;
    beginPreview(count);

    function raise(index: number) {
      const sample = sampleFor(index);
      sample.raise(sample.title, {
        description: t('notifications.previewBody', { index: index + 1, count }),
        position: value.position,
        richColors: value.richColors,
        closeButton: neverDismisses ? true : value.closeButton,
        duration: neverDismisses ? Infinity : value.durationMs,
      });
    }

    // The first sample is raised synchronously: a preview button that does
    // nothing for two seconds reads as broken.
    raise(0);
    for (let index = 1; index < count; index += 1) {
      timers.current.push(window.setTimeout(() => raise(index), index * PREVIEW_INTERVAL_MS));
    }

    // A little past the last sample's dismissal, so the override outlives the
    // toasts it was raised for rather than snapping back while they are still
    // on screen.
    const lastRaisedAt = (count - 1) * PREVIEW_INTERVAL_MS;
    timers.current.push(
      window.setTimeout(endPreview, lastRaisedAt + (neverDismisses ? PREVIEW_HOLD_MS : value.durationMs + 800)),
    );
  };
}

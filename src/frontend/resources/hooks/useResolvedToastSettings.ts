// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useAuth } from '@core/auth/useAuth';
import { useToastSettings } from '@resources/hooks/useToastSettings';
import type { ToastSettings } from '@resources/types/settings';
import type { ToastPosition } from '@core/auth/useAuth';

/**
 * What the shipped install does before anything is configured, and what the
 * toaster renders while the settings query is still in flight. These are the
 * values `main.tsx` used to hardcode, so a first paint looks identical to how
 * it always has.
 */
export const TOAST_FALLBACK: ToastSettings = {
  position: 'bottom-right',
  durationMs: 4000,
  maxVisible: 3,
  richColors: true,
  closeButton: true,
  allowUserOverride: true,
};

/** Everything the toaster needs, plus who decided it. */
export type ResolvedToastSettings = ToastSettings & {
  /** True when this account's own overrides are in effect. */
  overridden: boolean;
};

/**
 * Merges the app-wide toast defaults with this account's overrides.
 *
 * Two rules decide the answer:
 *
 *   1. An override only counts while the operator allows overrides at all.
 *      Revoking the permission does not delete anyone's choices, so a user's
 *      columns can be set and still not apply — the check is on the flag,
 *      not on whether values exist.
 *   2. An override is per-field. A user who only moved the toaster to the
 *      top keeps following the default duration, and keeps following it when
 *      the operator later changes that default.
 *
 * `undefined` is the only "not set": the backend omits an unset field rather
 * than sending null, which matters because 0 is a legal duration and false is
 * a legal richColors — neither can be treated as absent.
 */
export function useResolvedToastSettings(): ResolvedToastSettings {
  const user = useAuth((s) => s.user);
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  // Unauthenticated views (login, setup) still raise toasts, and the endpoint
  // is behind the session cookie — asking for it there would 401 on every
  // mount. They get the shipped defaults instead.
  const { data } = useToastSettings({ enabled: isAuthenticated });

  const base = data ?? TOAST_FALLBACK;
  if (!base.allowUserOverride || !user) {
    return { ...base, overridden: false };
  }

  const position: ToastPosition = user.toastPosition ?? base.position;
  const durationMs = user.toastDurationMs ?? base.durationMs;
  const maxVisible = user.toastMaxVisible ?? base.maxVisible;
  const richColors = user.toastRichColors ?? base.richColors;
  const closeButton = user.toastCloseButton ?? base.closeButton;

  return {
    position,
    durationMs,
    maxVisible,
    richColors,
    closeButton,
    allowUserOverride: base.allowUserOverride,
    overridden:
      user.toastPosition !== undefined ||
      user.toastDurationMs !== undefined ||
      user.toastMaxVisible !== undefined ||
      user.toastRichColors !== undefined ||
      user.toastCloseButton !== undefined,
  };
}

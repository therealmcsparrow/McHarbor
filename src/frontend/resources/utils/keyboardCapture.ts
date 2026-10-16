// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * Marks an area that owns every keystroke inside it — a remote console canvas, an
 * embedded terminal — so the app's global shortcuts stay silent there. Plugins set it
 * on their own elements; the host never needs to know which ones.
 */
export const KEYBOARD_CAPTURE_ATTR = 'data-keyboard-capture';

/** True when focus is in a field the user is typing into. */
export function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable === true;
}

/** True when the keystroke belongs to a capture zone, where no global shortcut may fire. */
export function isInCaptureZone(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest(`[${KEYBOARD_CAPTURE_ATTR}]`) !== null;
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type ConsoleDisplayMode = 'resize' | 'scale';

export const DISPLAY_MODE_STORAGE_KEY = 'mcharbor.kvm.console.displayMode';

/** Long enough that dragging a window edge sends one request, not sixty. */
export const RESIZE_DEBOUNCE_MS = 200;

/** How long a guest gets to answer a resize before the console says nothing did. */
export const GUEST_ANSWER_GRACE_MS = 5000;

/**
 * How often, and how many times, a VNC console asks again when the guest's
 * picture does not match its box. QEMU answers every request at once with
 * "forwarded", noVNC takes that as final, and a request made while it waited
 * was dropped, so the last size of a growing box (fullscreen) is never sent.
 */
export const RESIZE_RETRY_MS = 1500;
export const RESIZE_RETRIES = 3;

/** A per-viewer preference: a blocked or cleared store simply means the default. */
export function readDisplayMode(): ConsoleDisplayMode {
  try {
    return window.localStorage.getItem(DISPLAY_MODE_STORAGE_KEY) === 'scale' ? 'scale' : 'resize';
  } catch {
    return 'resize';
  }
}

export function writeDisplayMode(mode: ConsoleDisplayMode): void {
  try {
    window.localStorage.setItem(DISPLAY_MODE_STORAGE_KEY, mode);
  } catch {
    // Private windows and blocked site data refuse the write; the choice still holds for this page.
  }
}

/** Why the guest cannot follow the window, or null when nothing on the host side stops it. */
export type ResizeBlocker = 'adapter' | 'adapterQxl' | 'host' | null;

/**
 * Whether the guest's adapter and the host's QEMU can resize at all.
 *
 * VNC resizes through QEMU's ui_info, which only virtio-gpu implements, and
 * only from QEMU 6.0. SPICE resizes through spice-vdagent, which drives QXL
 * and virtio-gpu alike, so the host's version does not matter there. An
 * unknown adapter or capability (an older backend or agent) claims nothing.
 */
export function resizeBlocker(
  kind: 'vnc' | 'spice',
  adapter: string | undefined,
  hostResize: boolean | undefined,
): ResizeBlocker {
  if (!adapter) return null;
  if (kind === 'spice') return adapter === 'virtio-gpu' || adapter === 'qxl' ? null : 'adapter';
  if (adapter === 'qxl') return 'adapterQxl';
  if (adapter !== 'virtio-gpu') return 'adapter';
  return hostResize === false ? 'host' : null;
}

/**
 * The monitor size to ask a SPICE guest for: X needs both sides to be a
 * multiple of 8, and a collapsed container is not a size to ask for.
 */
export function spiceMonitorSize(width: number, height: number): { width: number; height: number } | null {
  const w = Math.floor(width / 8) * 8;
  const h = Math.floor(height / 8) * 8;
  return w >= 64 && h >= 64 ? { width: w, height: h } : null;
}

/** The scale that fits content inside a box, keeping its aspect ratio. */
export function fitScale(boxWidth: number, boxHeight: number, contentWidth: number, contentHeight: number): number {
  if (contentWidth <= 0 || contentHeight <= 0 || boxWidth <= 0 || boxHeight <= 0) return 1;
  return Math.min(boxWidth / contentWidth, boxHeight / contentHeight);
}

/** Whether a framebuffer is the size of the window it is shown in, within a scrollbar's slack. */
export function followsWindow(fbWidth: number, fbHeight: number, boxWidth: number, boxHeight: number): boolean {
  return Math.abs(fbWidth - boxWidth) <= 16 && Math.abs(fbHeight - boxHeight) <= 16;
}

export interface Debounced {
  call: () => void;
  cancel: () => void;
}

export function debounce(fn: () => void, wait: number): Debounced {
  let timer: ReturnType<typeof setTimeout> | undefined;
  return {
    call: () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = undefined;
        fn();
      }, wait);
    },
    cancel: () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
    },
  };
}

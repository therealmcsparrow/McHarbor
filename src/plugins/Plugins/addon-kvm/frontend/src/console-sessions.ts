// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import RFBModule from '@novnc/novnc/lib/rfb.js';
import { followsWindow, type ConsoleDisplayMode } from './console-resize';

// `lib/rfb.js` is Babel-transpiled CommonJS and esbuild's node-mode interop
// puts the whole exports object on `.default`, so the import lands one level
// above the class and `new RFB(...)` throws 'is not a constructor'.
// eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: CJS interop shape that noVNC's typings do not describe
const RFB = ((RFBModule as unknown as { default?: typeof RFBModule }).default ?? RFBModule) as typeof RFBModule;

type RFBInstance = InstanceType<typeof RFBModule>;

export type ConsoleStatus = 'idle' | 'connecting' | 'connected' | 'disconnected' | 'error';

export interface ConsoleSnapshot {
  status: ConsoleStatus;
  detail: string;
  viewers: number;
}

interface Session {
  container: HTMLDivElement;
  rfb: RFBInstance | null;
  generation: number;
  mode: ConsoleDisplayMode;
  snapshot: ConsoleSnapshot;
  listeners: Set<() => void>;
}

const IDLE: ConsoleSnapshot = { status: 'idle', detail: '', viewers: 0 };

const closeGrace = 1500;

const sessions = new Map<string, Session>();

function session(vmId: string): Session {
  let found = sessions.get(vmId);
  if (!found) {
    const container = document.createElement('div');
    container.className = 'h-full w-full';
    found = {
      container,
      rfb: null,
      generation: 0,
      mode: 'resize',
      snapshot: IDLE,
      listeners: new Set(),
    };
    sessions.set(vmId, found);
  }
  return found;
}

function update(vmId: string, patch: Partial<ConsoleSnapshot>): void {
  const current = session(vmId);
  current.snapshot = { ...current.snapshot, ...patch };
  for (const listener of current.listeners) listener();
}

let parkingLot: HTMLDivElement | null = null;

function parking(): HTMLDivElement {
  if (!parkingLot) {
    const lot = document.createElement('div');
    lot.setAttribute('aria-hidden', 'true');
    lot.style.position = 'fixed';
    lot.style.left = '-10000px';
    lot.style.top = '0';
    lot.style.width = '0';
    lot.style.height = '0';
    lot.style.pointerEvents = 'none';
    document.body.appendChild(lot);
    parkingLot = lot;
  }
  return parkingLot;
}

export function attachConsole(vmId: string, host: HTMLElement): void {
  const current = session(vmId);
  current.container.style.width = '';
  current.container.style.height = '';
  host.appendChild(current.container);
  nudgeConsole(vmId);
}

export function detachConsole(vmId: string): void {
  const current = sessions.get(vmId);
  if (!current) return;

  const rect = current.container.getBoundingClientRect();
  if (rect.width > 0 && rect.height > 0) {
    current.container.style.width = `${Math.round(rect.width)}px`;
    current.container.style.height = `${Math.round(rect.height)}px`;
  }
  parking().appendChild(current.container);
}

export function subscribeConsole(vmId: string, listener: () => void): () => void {
  const current = session(vmId);
  current.listeners.add(listener);
  return () => {
    current.listeners.delete(listener);
  };
}

export function consoleSnapshot(vmId: string): ConsoleSnapshot {
  return sessions.get(vmId)?.snapshot ?? IDLE;
}

export function consoleRFB(vmId: string): RFBInstance | null {
  return sessions.get(vmId)?.rfb ?? null;
}

export function isConsoleLive(vmId: string): boolean {
  const statusValue = consoleSnapshot(vmId).status;
  return statusValue === 'connected' || statusValue === 'connecting';
}

export function startConsole(vmId: string, url: string, viewers: number): void {
  const current = session(vmId);
  const generation = ++current.generation;
  update(vmId, { status: 'connecting', detail: '', viewers });

  current.container.replaceChildren();

  try {
    const rfb = new RFB(current.container, url, {});
    applyMode(rfb, current.mode);
    rfb.addEventListener('connect', () => {
      if (current.generation !== generation) return;
      update(vmId, { status: 'connected', detail: '' });
      nudgeConsole(vmId);
    });
    rfb.addEventListener('disconnect', (evt) => {
      if (current.generation !== generation) return;
      const clean = (evt as CustomEvent<{ clean?: boolean }>).detail?.clean;
      current.rfb = null;
      update(vmId, { status: clean ? 'disconnected' : 'error' });
    });
    current.rfb = rfb;
  } catch (error) {
    update(vmId, {
      status: 'error',
      detail: error instanceof Error ? error.message : String(error),
    });
  }
}

export function failConsole(vmId: string, detail: string): void {
  update(vmId, { status: 'error', detail });
}

export function closeConsole(vmId: string): Promise<void> {
  const current = sessions.get(vmId);
  if (!current) return Promise.resolve();
  return teardown(current);
}

export function stopConsole(vmId: string): void {
  const current = sessions.get(vmId);
  if (!current) return;
  void teardown(current);
  update(vmId, { status: 'idle', detail: '', viewers: 0 });
}

function teardown(current: Session): Promise<void> {
  const rfb = current.rfb;
  if (!rfb) return Promise.resolve();

  current.rfb = null;
  current.generation += 1;

  return new Promise<void>((resolve) => {
    let timer = 0;
    let settled = false;
    const done = () => {
      if (settled) return;
      settled = true;
      window.clearTimeout(timer);
      resolve();
    };

    rfb.addEventListener('disconnect', done);
    timer = window.setTimeout(done, closeGrace);
    try {
      rfb.disconnect();
    } catch {
      done();
    }
  });
}

/**
 * The two noVNC switches a display mode sets.
 *
 * Scaling stays on in both modes: while a guest is still changing resolution,
 * or when it never will, the picture fits rather than overflowing. Only the
 * resize mode asks the guest to change (ExtendedDesktopSize / SetDesktopSize).
 */
function applyMode(rfb: RFBInstance, mode: ConsoleDisplayMode): void {
  rfb.scaleViewport = true;
  rfb.resizeSession = mode === 'resize';
}

/** Sets how one console fits its window; a live session switches at once. */
export function setConsoleDisplayMode(vmId: string, mode: ConsoleDisplayMode): void {
  const current = session(vmId);
  if (current.mode === mode) return;
  current.mode = mode;
  nudgeConsole(vmId);
}

/**
 * Whether the guest's framebuffer is the size of its window, or null when
 * there is no live picture to compare. noVNC sizes its canvas to the
 * framebuffer and scales it with CSS, so the canvas attributes are the guest's.
 */
export function consoleFollowsWindow(vmId: string): boolean | null {
  const current = sessions.get(vmId);
  if (!current?.rfb || current.snapshot.status !== 'connected') return null;
  const canvas = current.container.querySelector('canvas');
  const box = current.container.getBoundingClientRect();
  if (!canvas || box.width <= 0 || box.height <= 0) return null;
  return followsWindow(canvas.width, canvas.height, box.width, box.height);
}

/**
 * Make noVNC recompute its scale.
 *
 * `scaleViewport` recalculates on the window's resize event and nothing else —
 * it does not watch the element. The console container settles to its real
 * height after the socket connects, which fires no window resize, so the canvas
 * kept the size it had at connect time and the screen looked blank until a
 * fullscreen round trip finally produced one. Re-assigning the two properties
 * reaches one connection where a window event would reach every parked one too.
 */
export function nudgeConsole(vmId: string): void {
  const current = sessions.get(vmId);
  if (!current?.rfb || current.snapshot.status !== 'connected') return;
  const rfb = current.rfb;

  requestAnimationFrame(() => {
    if (current.rfb !== rfb) return;
    try {
      applyMode(rfb, current.mode);
    } catch {
      // noVNC throws when the session closed between the frame request and here; the next connect re-applies both.
    }
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const session = vi.hoisted(() => ({
  nudges: 0,
  follows: [] as Array<boolean | null>,
}));

vi.mock('../../../plugins/Plugins/addon-kvm/frontend/src/console-sessions', () => ({
  nudgeConsole: () => {
    session.nudges += 1;
  },
  setConsoleDisplayMode: () => {},
  consoleFollowsWindow: () => (session.follows.length > 1 ? session.follows.shift() : session.follows[0]) ?? null,
}));

import { useVncResize } from '../../../plugins/Plugins/addon-kvm/frontend/src/hooks/useVncResize';
import {
  RESIZE_DEBOUNCE_MS,
  RESIZE_RETRIES,
  RESIZE_RETRY_MS,
} from '../../../plugins/Plugins/addon-kvm/frontend/src/console-resize';

function mount() {
  const screenRef = { current: document.createElement('div') };
  return renderHook(() => useVncResize('vm1', screenRef, 'connected', 'resize', false));
}

describe('addon-kvm VNC resize retries', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    session.nudges = 0;
    session.follows = [];
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  // QEMU answers "forwarded" at once and noVNC stops there, so the final size
  // of a growing box (fullscreen) only reaches the guest because we ask again.
  it('asks again while the picture does not match the box, and stops once it does', () => {
    session.follows = [false, true];
    const { result } = mount();

    void act(() => vi.advanceTimersByTime(RESIZE_DEBOUNCE_MS));
    expect(session.nudges).toBe(1);
    void act(() => vi.advanceTimersByTime(RESIZE_RETRY_MS));
    expect(session.nudges).toBe(2);
    void act(() => vi.advanceTimersByTime(RESIZE_RETRY_MS * 3));
    expect(session.nudges).toBe(2);
    expect(result.current).toBe(false);
  });

  it('reports a guest that never follows only after the last retry', () => {
    session.follows = [false];
    const { result } = mount();

    void act(() => vi.advanceTimersByTime(RESIZE_DEBOUNCE_MS + RESIZE_RETRY_MS * RESIZE_RETRIES));
    expect(session.nudges).toBe(1 + RESIZE_RETRIES);
    expect(result.current).toBe(false);
    void act(() => vi.advanceTimersByTime(RESIZE_RETRY_MS));
    expect(session.nudges).toBe(1 + RESIZE_RETRIES);
    expect(result.current).toBe(true);
  });
});

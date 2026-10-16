// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useFlash, useFlashValue } from './useFlash';

describe('useFlash', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('flashes true and reverts after the duration', () => {
    const { result } = renderHook(() => useFlash(2000));
    expect(result.current.flashing).toBe(false);
    act(() => result.current.flash());
    expect(result.current.flashing).toBe(true);
    act(() => void vi.advanceTimersByTime(1999));
    expect(result.current.flashing).toBe(true);
    act(() => void vi.advanceTimersByTime(1));
    expect(result.current.flashing).toBe(false);
  });

  it('restarts the countdown on a re-flash instead of reverting on the first timer', () => {
    const { result } = renderHook(() => useFlash(2000));
    act(() => result.current.flash());
    act(() => void vi.advanceTimersByTime(1500));
    act(() => result.current.flash());
    act(() => void vi.advanceTimersByTime(1500));
    expect(result.current.flashing).toBe(true);
    act(() => void vi.advanceTimersByTime(500));
    expect(result.current.flashing).toBe(false);
  });

  it('clear reverts at once and leaves no timer behind', () => {
    const { result } = renderHook(() => useFlash(2000));
    act(() => result.current.flash());
    act(() => result.current.clear());
    expect(result.current.flashing).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('clears the pending timer on unmount', () => {
    const { result, unmount } = renderHook(() => useFlash(2000));
    act(() => result.current.flash());
    expect(vi.getTimerCount()).toBe(1);
    unmount();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('keeps the latest value when a keyed flash is replaced', () => {
    const { result } = renderHook(() => useFlashValue<string>(2000));
    act(() => result.current.flash('a'));
    act(() => result.current.flash('b'));
    expect(result.current.value).toBe('b');
    act(() => void vi.advanceTimersByTime(2000));
    expect(result.current.value).toBeNull();
  });
});

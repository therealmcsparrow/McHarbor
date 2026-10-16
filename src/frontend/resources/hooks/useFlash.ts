// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useRef, useState } from 'react';

/**
 * useFlashValue holds a transient value that reverts to null after duration ms.
 * The timer lives in a ref so a re-flash restarts the countdown instead of
 * letting the first timeout cut the second flash short, and unmount clears it.
 */
export function useFlashValue<T>(duration = 2000) {
  const [value, setValue] = useState<T | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const clear = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = undefined;
    setValue(null);
  }, []);

  const flash = useCallback(
    (next: T) => {
      clearTimeout(timer.current);
      setValue(next);
      timer.current = setTimeout(() => {
        timer.current = undefined;
        setValue(null);
      }, duration);
    },
    [duration],
  );

  useEffect(() => () => clearTimeout(timer.current), []);

  return { value, flash, clear };
}

/** useFlash is the boolean form of useFlashValue, for a "Copied" affordance that fades back on its own. */
export function useFlash(duration = 2000) {
  const { value, flash, clear } = useFlashValue<true>(duration);
  const raise = useCallback(() => flash(true), [flash]);
  return { flashing: value !== null, flash: raise, clear };
}

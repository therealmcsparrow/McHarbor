// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';

/**
 * useLockoutCountdown ticks `seconds` down to zero once a second.
 *
 * Starts at the value of `initialSeconds` and stops when it hits
 * zero. The hook returns the live remaining time AND a `done`
 * boolean so callers can re-enable the form the moment the lock
 * expires.
 */
export function useLockoutCountdown(initialSeconds: number | undefined) {
  const [remaining, setRemaining] = useState(initialSeconds ?? 0);
  const [trackedSeconds, setTrackedSeconds] = useState(initialSeconds);

  if (trackedSeconds !== initialSeconds) {
    setTrackedSeconds(initialSeconds);
    setRemaining(initialSeconds ?? 0);
  }

  useEffect(() => {
    if (!remaining || remaining <= 0) return;
    const id = window.setInterval(() => {
      setRemaining((s) => (s > 0 ? s - 1 : 0));
    }, 1000);
    return () => window.clearInterval(id);
  }, [remaining]);

  return {
    remaining,
    done: remaining <= 0,
  };
}

export function formatCountdown(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m === 0) return `${s}s`;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

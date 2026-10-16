// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';
import { api } from '@core/api/client';
import { useAuth } from '@core/auth/useAuth';

type SessionPolicy = {
  /** Seconds of inactivity before sign-out. 0 disables the feature. */
  idleTimeoutSeconds: number;
};

/**
 * Events that count as "the user is still here". Deliberately input
 * events only: a background refetch, a websocket frame, or a rerender
 * are the *app* being busy, which is exactly what an idle timeout has
 * to be able to see past.
 *
 * `visibilitychange` is *not* here. Returning to a tab is intentional,
 * but by then the server may already have expired the session, and
 * treating the return as activity would reset the local clock and leave
 * the page sitting on a session that is gone. It gets its own listener
 * that re-checks the deadline instead.
 */
const ACTIVITY_EVENTS = ['mousedown', 'mousemove', 'keydown', 'wheel', 'touchstart', 'scroll'] as const;

/**
 * How often, at most, activity is reported to the server. The server
 * only needs to know the user was here more recently than the window;
 * one write a minute is enough resolution for a window that is at least
 * sixty seconds wide.
 */
const TOUCH_INTERVAL_MS = 60_000;

/** How often the local countdown is compared against the deadline. */
const CHECK_INTERVAL_MS = 5_000;

/**
 * Grace period added to the local deadline. The server is the authority;
 * the client deliberately fires a few seconds late so a clock skew of a
 * second or two cannot sign someone out while their session is still
 * perfectly valid server-side.
 */
const CLIENT_GRACE_MS = 3_000;

/**
 * Signs the user out after a configurable period of inactivity.
 *
 * Two halves that have to agree:
 *
 *  - The **server** expires any session whose `last_activity_at` is
 *    older than the window. That is the boundary that actually matters;
 *    defeating anything below in devtools buys nothing.
 *  - This hook keeps that column fresh while the user is genuinely
 *    active, and runs the same countdown locally so an idle tab is
 *    redirected to /login instead of sitting on a dead page until the
 *    next click returns a 401.
 *
 * A zero window disables both halves. Mounted once, in AuthProvider.
 */
export function useIdleLogout() {
  const isAuthenticated = useAuth((s) => s.isAuthenticated);
  const logout = useAuth((s) => s.logout);

  // Refs, not state: every one of these changes on mousemove, and
  // re-rendering the whole app on mousemove would be a bug of its own.
  const lastActivityRef = useRef(0);
  const lastTouchRef = useRef(0);
  const loggingOutRef = useRef(false);

  useEffect(() => {
    if (!isAuthenticated) return;

    lastActivityRef.current = Date.now();
    let cancelled = false;
    let timeoutMs = 0;
    let checkTimer: number | undefined;

    const markActivity = () => {
      if (document.visibilityState === 'hidden') return;
      lastActivityRef.current = Date.now();
    };

    /**
     * Browsers throttle timers in a hidden tab, so the countdown may not
     * have run while the user was away. Re-check on return rather than
     * waiting up to CHECK_INTERVAL_MS to notice the session is over.
     */
    const onVisibilityChange = () => {
      if (document.visibilityState !== 'visible') return;
      if (checkTimer !== undefined) window.clearTimeout(checkTimer);
      tick();
    };

    /**
     * Report activity to the server, throttled. The reply carries the
     * current window, so an admin who tightens or lifts the policy
     * reaches every open tab within a minute without a reload.
     *
     * Failures are swallowed: a dropped keepalive is not worth a toast,
     * and if the session is genuinely gone the next real request
     * answers 401 anyway.
     */
    const touch = () => {
      const now = Date.now();
      if (now - lastTouchRef.current < TOUCH_INTERVAL_MS) return;
      lastTouchRef.current = now;
      void api
        .post<SessionPolicy>('/auth/session/touch')
        .then((res) => {
          if (cancelled || !res.success || !res.data) return;
          timeoutMs = Math.max(0, res.data.idleTimeoutSeconds) * 1000;
        })
        .catch(() => undefined);
    };

    const signOut = () => {
      if (loggingOutRef.current) return;
      loggingOutRef.current = true;
      logout();
    };

    const tick = () => {
      if (cancelled || timeoutMs <= 0) return;

      const idleFor = Date.now() - lastActivityRef.current;
      if (idleFor >= timeoutMs + CLIENT_GRACE_MS) {
        signOut();
        return;
      }

      // Only keep the server's clock moving while the user is actually
      // here. Touching on a timer regardless would mean an abandoned
      // tab renews its own session forever, which is the one outcome
      // this feature exists to prevent.
      if (idleFor < TOUCH_INTERVAL_MS) touch();

      checkTimer = window.setTimeout(tick, CHECK_INTERVAL_MS);
    };

    void (async () => {
      const res = await api.get<SessionPolicy>('/auth/session/policy');
      if (cancelled) return;

      const seconds = res.success ? (res.data?.idleTimeoutSeconds ?? 0) : 0;
      if (seconds <= 0) return;

      timeoutMs = seconds * 1000;
      lastActivityRef.current = Date.now();
      lastTouchRef.current = Date.now();

      for (const evt of ACTIVITY_EVENTS) {
        window.addEventListener(evt, markActivity, { passive: true });
      }
      document.addEventListener('visibilitychange', onVisibilityChange);
      checkTimer = window.setTimeout(tick, CHECK_INTERVAL_MS);
    })();

    return () => {
      cancelled = true;
      if (checkTimer !== undefined) window.clearTimeout(checkTimer);
      for (const evt of ACTIVITY_EVENTS) {
        window.removeEventListener(evt, markActivity);
      }
      document.removeEventListener('visibilitychange', onVisibilityChange);
    };
  }, [isAuthenticated, logout]);
}

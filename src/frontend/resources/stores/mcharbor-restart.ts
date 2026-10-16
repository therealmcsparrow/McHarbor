// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { ApiResponse } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

/**
 * Unwraps the restart request. A native install nothing supervises is refused with reason
 * `not_supervised`; the server's generic code still speaks of a Docker container, which is the
 * wrong advice there, so the caller's own message replaces it.
 */
export function assertRestartScheduled<T>(res: ApiResponse<T>, notSupervisedMessage: string): T {
  const reason = (res.data as { reason?: unknown } | undefined)?.reason;
  if (!res.success && reason === 'not_supervised') {
    throw new Error(notSupervisedMessage);
  }
  return assertSuccess(res);
}

export type McHarborRestartPhase =
  | 'preparing'
  | 'scheduled'
  | 'stopping'
  | 'starting'
  | 'verifying'
  | 'ready'
  | 'error';

type RestartState = {
  open: boolean;
  phase: McHarborRestartPhase;
  progress: number;
  error: string | null;
  begin: () => void;
  setPhase: (phase: McHarborRestartPhase, progress: number) => void;
  fail: (message: string) => void;
  reset: () => void;
};

type HealthSnapshot = {
  status: string;
  instanceID: string;
};

const recoveryTimeoutMs = 120_000;
const pollIntervalMs = 1_000;
const healthRequestTimeoutMs = 3_000;
export const restartRecoveryTimeoutError = 'restart-recovery-timeout';
export const restartUnknownError = 'restart-unknown-error';

export const useMcHarborRestartStore = create<RestartState>((set) => ({
  open: false,
  phase: 'preparing',
  progress: 0,
  error: null,
  begin: () => set({ open: true, phase: 'preparing', progress: 10, error: null }),
  setPhase: (phase, progress) => set({ phase, progress }),
  fail: (message) => set({ open: true, phase: 'error', progress: 100, error: message }),
  reset: () => set({ open: false, phase: 'preparing', progress: 0, error: null }),
}));

function delay(milliseconds: number) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function failureCode(error: unknown) {
  return error instanceof Error && error.message === restartRecoveryTimeoutError
    ? restartRecoveryTimeoutError
    : restartUnknownError;
}

function isHealthSnapshot(value: unknown): value is HealthSnapshot {
  if (!value || typeof value !== 'object') return false;
  const currentStatus = Reflect.get(value, 'status');
  const instanceID = Reflect.get(value, 'instanceID');
  return typeof currentStatus === 'string' && typeof instanceID === 'string' && instanceID.length > 0;
}

async function readHealth(): Promise<HealthSnapshot | null> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), healthRequestTimeoutMs);
  try {
    const response = await fetch('/api/health', {
      cache: 'no-store',
      credentials: 'include',
      signal: controller.signal,
    });
    if (!response.ok) return null;
    const payload: unknown = await response.json();
    if (!payload || typeof payload !== 'object') return null;
    const data = Reflect.get(payload, 'data');
    if (!data || typeof data !== 'object') return null;
    const snapshot = {
      status: Reflect.get(data, 'status'),
      instanceID: Reflect.get(data, 'instanceId'),
    };
    return isHealthSnapshot(snapshot) && snapshot.status === 'ok' ? snapshot : null;
  } catch {
    return null;
  } finally {
    clearTimeout(timeout);
  }
}

async function waitForRecovery(baseline: HealthSnapshot) {
  const deadline = Date.now() + recoveryTimeoutMs;
  let recoveredInstanceID: string | null = null;
  let stableResponses = 0;

  while (Date.now() < deadline) {
    const health = await readHealth();
    if (!health) {
      recoveredInstanceID = null;
      stableResponses = 0;
      useMcHarborRestartStore.getState().setPhase('starting', 70);
    } else if (health.instanceID !== baseline.instanceID) {
      stableResponses = health.instanceID === recoveredInstanceID ? stableResponses + 1 : 1;
      recoveredInstanceID = health.instanceID;
      useMcHarborRestartStore.getState().setPhase('verifying', 90);
      if (stableResponses >= 2) return;
    } else {
      recoveredInstanceID = null;
      stableResponses = 0;
      useMcHarborRestartStore.getState().setPhase('stopping', 45);
    }
    await delay(pollIntervalMs);
  }

  throw new Error(restartRecoveryTimeoutError);
}

async function recoverAndReload(baseline: HealthSnapshot) {
  await waitForRecovery(baseline);
  useMcHarborRestartStore.getState().setPhase('ready', 100);
  await delay(1_000);
  window.location.reload();
}

export async function runWithMcHarborRestart<T>(request: () => Promise<T>): Promise<T | undefined> {
  const store = useMcHarborRestartStore.getState();
  store.begin();
  const baseline = await readHealth();
  if (!baseline) {
    const error = new Error(restartUnknownError);
    useMcHarborRestartStore.getState().fail(restartUnknownError);
    throw error;
  }

  try {
    const result = await request();
    useMcHarborRestartStore.getState().setPhase('scheduled', 30);
    void recoverAndReload(baseline).catch((error: unknown) => {
      useMcHarborRestartStore.getState().fail(failureCode(error));
    });
    return result;
  } catch (error) {
    if (!(error instanceof TypeError) && !(error instanceof SyntaxError)) {
      useMcHarborRestartStore.getState().fail(restartUnknownError);
      throw error;
    }
  }

  try {
    useMcHarborRestartStore.getState().setPhase('scheduled', 30);
    await recoverAndReload(baseline);
    return undefined;
  } catch (error) {
    useMcHarborRestartStore.getState().fail(failureCode(error));
    throw error;
  }
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { setupRefusal, setupTokenHeaders } from '@core/auth/setupToken';
import {
  ACTIVE_MIGRATION_PHASES,
  type CleanupResult,
  type CleanupStatus,
  type DatabaseConfig,
  type DatabaseConnectionInput,
  type DatabaseSetupApplyResult,
  type DatabaseSetupStatus,
  type DatabaseTestResult,
  type MigrationStatus,
} from '../types';

/** The database McHarbor is running on right now. */
export function useDatabaseConfig() {
  return useQuery({
    queryKey: ['database', 'config'],
    queryFn: () => api.get<DatabaseConfig>('/database/config').then(assertSuccess),
  });
}

/**
 * The step-by-step connection test.
 *
 * A failed test is a successful request: the seven steps and which one
 * stopped are the answer, not an error. So the mutation only rejects when
 * the request itself failed, and the caller reads `ok` off the result.
 */
export function useDatabaseTest() {
  return useMutation({
    mutationFn: (input: DatabaseConnectionInput) =>
      api.post<DatabaseTestResult>('/database/test', input).then(assertSuccess),
  });
}

/** The same test, on the first-run route that works before any account exists. */
export function useDatabaseSetupTest() {
  return useMutation({
    mutationFn: (input: DatabaseConnectionInput) =>
      api.post<DatabaseTestResult>('/database/setup/test', input, { headers: setupTokenHeaders() }).then(setupRefusal),
  });
}

/**
 * The first-run database status.
 *
 * `enabled` exists so the signed-in application never asks: the endpoint only
 * has something to say while no account exists, and the two callers that read
 * it (the root redirect and the admin form) already know whether that is the
 * case.
 */
export function useDatabaseSetupStatus(enabled = true) {
  return useQuery({
    queryKey: ['database', 'setup', 'status'],
    queryFn: () => api.get<DatabaseSetupStatus>('/database/setup/status').then(assertSuccess),
    enabled,
    retry: false,
  });
}

export function useApplyDatabaseSetup() {
  return useMutation({
    mutationFn: (input: DatabaseConnectionInput) =>
      api
        .post<DatabaseSetupApplyResult>('/database/setup/apply', input, { headers: setupTokenHeaders() })
        .then(setupRefusal),
  });
}

export function useStartMigration() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: { target: DatabaseConnectionInput; restartAfter: boolean }) =>
      api
        .post<MigrationStatus>('/database/migration', {
          target: input.target,
          restartAfter: input.restartAfter,
        })
        .then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['database', 'migration'] });
    },
  });
}

export function useCancelMigration() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post('/database/migration/cancel', {}).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['database', 'migration'] });
    },
  });
}

/**
 * The migration job, polled while it runs.
 *
 * Polling rather than a stream: the payload is small, the job is minutes
 * long at most, and a poll survives the connection drop that the container
 * restart at the end of a successful migration causes anyway.
 *
 * `enabled` is what the modal toggles — there is no reason to poll a job
 * nobody is looking at, and a 404 (no job has ever run in this process) is
 * a normal answer rather than a failure to retry.
 */
export function useMigrationStatus(enabled: boolean) {
  return useQuery({
    queryKey: ['database', 'migration'],
    queryFn: () => api.get<MigrationStatus>('/database/migration').then((r) => (r.success ? (r.data ?? null) : null)),
    enabled,
    retry: false,
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return 2_000;
      return ACTIVE_MIGRATION_PHASES.includes(data.phase) ? 1_000 : false;
    },
  });
}

/**
 * The old SQLite file a completed migration left on disk.
 *
 * Read on the settings page rather than on demand, because the card only
 * appears when there is something to delete — and an operator should see that
 * offer without having to go looking for it.
 */
export function useDatabaseCleanup() {
  return useQuery({
    queryKey: ['database', 'cleanup'],
    queryFn: () => api.get<CleanupStatus>('/database/cleanup').then(assertSuccess),
    retry: false,
  });
}

export function useRemovePreviousDatabase() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<CleanupResult>('/database/cleanup', {}).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['database', 'cleanup'] });
      void queryClient.invalidateQueries({ queryKey: ['database', 'config'] });
    },
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import * as api from '../api';
import type { GenerateRequest, ScheduleRequest, Settings, SubjectKind } from '../types';

// Server state lives in TanStack Query, never in component state.
//
// Report generation is the slow call here — it fans out across a dozen
// host endpoints — so it is a mutation with an explicit pending state
// the studio can show, not a query that would re-run on focus.

const keys = {
  catalog: ['reporting', 'catalog'] as const,
  reports: (page: number, type?: string) => ['reporting', 'reports', page, type ?? ''] as const,
  report: (id: string) => ['reporting', 'report', id] as const,
  settings: ['reporting', 'settings'] as const,
  aiConnections: ['reporting', 'ai-connections'] as const,
  subjects: (kind: SubjectKind | null, envId: string) => ['reporting', 'subjects', kind ?? '', envId] as const,
  job: (id: string) => ['reporting', 'job', id] as const,
  schedules: (page: number) => ['reporting', 'schedules', page] as const,
  scheduleRun: (id: string) => ['reporting', 'schedule-run', id] as const,
  deliveryTargets: ['reporting', 'delivery-targets'] as const,
};

export function useCatalog() {
  return useQuery({
    queryKey: keys.catalog,
    queryFn: api.fetchCatalog,
    // The catalog is compiled into the plugin; it cannot change while
    // the page is open.
    staleTime: Infinity,
  });
}

export function useReports(page: number, perPage: number, type?: string) {
  return useQuery({
    queryKey: keys.reports(page, type),
    queryFn: () => api.fetchReports(page, perPage, type),
  });
}

export function useReport(id: string | null) {
  return useQuery({
    queryKey: keys.report(id ?? ''),
    queryFn: () => api.fetchReport(id as string),
    enabled: Boolean(id),
    // A stored report is immutable once written, so it never needs
    // refetching while the viewer is open.
    staleTime: Infinity,
  });
}

/**
 * The objects the chosen report may be narrowed to.
 *
 * Disabled until a kind is chosen, so opening the configure dialog for a
 * report that offers no scope costs nothing. The list is per environment
 * because a container in one environment is not a container in another.
 */
export function useSubjects(kind: SubjectKind | null, envId: string) {
  return useQuery({
    queryKey: keys.subjects(kind, envId),
    queryFn: () => api.fetchSubjects(kind as SubjectKind, envId),
    enabled: kind !== null,
    // The picker is opened, read and closed inside a few seconds; a
    // refetch on focus would reorder the list under the pointer.
    staleTime: 30_000,
  });
}

/** Start a generation. The job id is what the progress dialog watches. */
export function useStartGenerateJob() {
  return useMutation({
    mutationFn: (body: GenerateRequest) => api.startGenerateJob(body),
  });
}

/**
 * Poll one generation until it settles.
 *
 * A second is the right cadence: collection is the slow part and reports
 * a source at a time, so the bar moves on most ticks without the poll
 * itself becoming load on the host it is measuring.
 */
export function useGenerateJob(id: string | null) {
  const qc = useQueryClient();
  return useQuery({
    queryKey: keys.job(id ?? ''),
    queryFn: async () => {
      const job = await api.fetchGenerateJob(id as string);
      if (job.state === 'done') {
        void qc.invalidateQueries({ queryKey: ['reporting', 'reports'] });
      }
      return job;
    },
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      return state === 'done' || state === 'failed' ? false : 1000;
    },
  });
}

export function useDeleteReport() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteReport(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['reporting', 'reports'] });
    },
  });
}

// --- schedules ---------------------------------------------------------

/**
 * The saved schedules.
 *
 * Polled every fifteen seconds rather than left static: a schedule can
 * fire from its cron while the page is open, and a table that still shows
 * last week's run after tonight's has finished is worse than no table.
 */
export function useSchedules(page: number, perPage: number) {
  return useQuery({
    queryKey: keys.schedules(page),
    queryFn: () => api.fetchSchedules(page, perPage),
    refetchInterval: 15_000,
  });
}

/** The stored last run, with its document. Fetched only when opened. */
export function useScheduleRun(id: string | null) {
  return useQuery({
    queryKey: keys.scheduleRun(id ?? ''),
    queryFn: () => api.fetchScheduleRun(id as string),
    enabled: Boolean(id),
  });
}

/**
 * The channels and storage locations a schedule may deliver to.
 *
 * Fetched once and reused for the life of the dialog: which destinations
 * exist does not change while a schedule is being configured, and a
 * refetch on focus would reorder the list under the pointer.
 */
export function useDeliveryTargets(enabled: boolean) {
  return useQuery({
    queryKey: keys.deliveryTargets,
    queryFn: api.fetchDeliveryTargets,
    enabled,
    staleTime: 30_000,
  });
}

export function useCreateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: ScheduleRequest) => api.createSchedule(body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['reporting', 'schedules'] });
    },
  });
}

export function useUpdateSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: ScheduleRequest }) => api.updateSchedule(id, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['reporting', 'schedules'] });
    },
  });
}

export function useDeleteSchedule() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteSchedule(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['reporting', 'schedules'] });
    },
  });
}

export function useRunScheduleNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.runScheduleNow(id),
    onSuccess: (_data, id) => {
      // The run is asynchronous, so there is nothing to show yet — the
      // fifteen-second poll picks the outcome up. Invalidating here only
      // flips the row to "running" a poll sooner.
      void qc.invalidateQueries({ queryKey: ['reporting', 'schedules'] });
      void qc.invalidateQueries({ queryKey: keys.scheduleRun(id) });
    },
  });
}

export function useExportScheduleRun() {
  return useMutation({
    mutationFn: ({ id, format, name: itemName }: { id: string; format: 'pdf' | 'docx'; name: string }) =>
      api.downloadScheduleRun(id, format, itemName),
  });
}

export function useSettings() {
  return useQuery({ queryKey: keys.settings, queryFn: api.fetchSettings });
}

/**
 * The AI connections the configure dialog may offer.
 *
 * Fetched once and reused for the life of the dialog: which connections
 * exist does not change while a report is being configured, and a
 * refetch on focus would reorder the list under the selector.
 */
export function useAIConnections() {
  return useQuery({
    queryKey: keys.aiConnections,
    queryFn: api.fetchAIConnections,
    staleTime: 30_000,
  });
}

export function useSaveSettings() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Settings) => api.saveSettings(body),
    onSuccess: (saved) => {
      qc.setQueryData(keys.settings, saved);
      void qc.invalidateQueries({ queryKey: keys.catalog });
    },
  });
}

export function useExport() {
  return useMutation({
    mutationFn: ({ id, format, name: itemName }: { id: string; format: 'pdf' | 'docx'; name: string }) =>
      api.downloadExport(id, format, itemName),
  });
}

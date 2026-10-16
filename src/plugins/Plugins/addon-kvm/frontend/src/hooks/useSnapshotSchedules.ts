// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { ScheduleInput, SnapshotScheduleView } from '../types';

const KEYS = {
  schedules: (id: string) => ['vms', 'snapshot-schedules', id] as const,
};

export function useSnapshotSchedules(id: string | undefined) {
  return useQuery({
    queryKey: KEYS.schedules(id ?? ''),
    enabled: Boolean(id),
    queryFn: async () => assertSuccess(await api.get<SnapshotScheduleView>(`/vms/${id}/snapshot-schedules`)),
  });
}

function useScheduleMutation<TInput extends { id: string }, TResult>(run: (input: TInput) => Promise<TResult>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: run,
    onSettled: (_data, _error, input) => {
      void queryClient.invalidateQueries({ queryKey: KEYS.schedules(input.id) });
      void queryClient.invalidateQueries({ queryKey: ['vms', 'snapshots', input.id] });
    },
  });
}

export function useCreateSchedule() {
  return useScheduleMutation(async (input: { id: string } & ScheduleInput) => {
    const { id, ...body } = input;
    return assertSuccess(await api.post(`/vms/${id}/snapshot-schedules`, body));
  });
}

export function useUpdateSchedule() {
  return useScheduleMutation(async (input: { id: string; scheduleId: string } & ScheduleInput) => {
    const { id, scheduleId, ...body } = input;
    return assertSuccess(await api.put(`/vms/${id}/snapshot-schedules/${scheduleId}`, body));
  });
}

export function useDeleteSchedule() {
  return useScheduleMutation(async (input: { id: string; scheduleId: string }) =>
    assertSuccess(await api.del(`/vms/${input.id}/snapshot-schedules/${input.scheduleId}`)),
  );
}

export function useRunSchedule() {
  return useScheduleMutation(async (input: { id: string; scheduleId: string }) =>
    assertSuccess(await api.post(`/vms/${input.id}/snapshot-schedules/${input.scheduleId}/run`, {})),
  );
}

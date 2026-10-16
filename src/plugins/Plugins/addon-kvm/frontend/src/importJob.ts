// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useSyncExternalStore } from 'react';
import type { QueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { apiErrorOf, assertSuccess } from '@resources/utils/api-mutation';
import type { ImportConvertInput, VM, VMImport } from './types';

export type ImportPhase =
  | 'idle'
  | 'uploading'
  | 'uploaded'
  | 'inspecting'
  | 'inspected'
  | 'converting'
  | 'done'
  | 'failed';

export interface ImportJob {
  phase: ImportPhase;
  fileName: string;
  percent: number;
  record: VMImport | null;
  environmentId: string;
  vmId: string;
  error: string;
}

const IDLE: ImportJob = {
  phase: 'idle',
  fileName: '',
  percent: 0,
  record: null,
  environmentId: '',
  vmId: '',
  error: '',
};

let job: ImportJob = IDLE;
const listeners = new Set<() => void>();

function emit(): void {
  listeners.forEach((listener) => listener());
}

function set(patch: Partial<ImportJob>): void {
  job = { ...job, ...patch };
  emit();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function snapshot(): ImportJob {
  return job;
}

export function useImportJob(): ImportJob {
  return useSyncExternalStore(subscribe, snapshot, snapshot);
}

export function isImportRunning(phase: ImportPhase): boolean {
  return phase === 'uploading' || phase === 'inspecting' || phase === 'converting';
}

export function adoptImportJob(record: VMImport): void {
  if (isImportRunning(job.phase)) return;
  set({
    ...IDLE,
    phase: record.inspection ? 'inspected' : 'uploaded',
    fileName: record.fileName,
    percent: 100,
    record,
    environmentId: record.environmentId ?? '',
  });
}

export function dismissImportJob(): void {
  if (isImportRunning(job.phase)) return;
  job = IDLE;
  emit();
}

function uploadRequest(file: File, environmentId: string, onProgress: (percent: number) => void): Promise<VMImport> {
  return new Promise<VMImport>((resolve, reject) => {
    const body = new FormData();
    body.append('file', file, file.name);

    const request = new XMLHttpRequest();
    request.open('POST', `/api/vms/imports?environmentId=${encodeURIComponent(environmentId)}`);
    request.withCredentials = true;
    request.upload.addEventListener('progress', (evt) => {
      if (!evt.lengthComputable) return;
      onProgress(Math.round((evt.loaded / evt.total) * 100));
    });
    request.addEventListener('load', () => {
      let envelope: {
        success?: boolean;
        data?: VMImport;
        code?: string;
        error?: string;
        detail?: string;
        requestId?: string;
      } = {};
      try {
        envelope = JSON.parse(request.responseText) as typeof envelope;
      } catch {
        reject(new Error(request.statusText || 'upload failed'));
        return;
      }
      if (request.status >= 400 || !envelope.success || !envelope.data) {
        reject(apiErrorOf({ ...envelope, success: false, message: request.statusText }));
        return;
      }
      resolve(envelope.data);
    });
    request.addEventListener('error', () => reject(new Error('upload failed')));
    request.addEventListener('abort', () => reject(new Error('upload cancelled')));
    request.send(body);
  });
}

export function startImportUpload(file: File, environmentId: string, queryClient: QueryClient): void {
  if (isImportRunning(job.phase)) return;
  set({ ...IDLE, phase: 'uploading', fileName: file.name, environmentId });
  void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });

  void uploadRequest(file, environmentId, (percent) => set({ percent }))
    .then((record) =>
      set({
        phase: 'uploaded',
        percent: 100,
        record,
        environmentId: record.environmentId ?? environmentId,
      }),
    )
    .catch((error: Error) => set({ phase: 'failed', error: error.message }))
    .finally(() => {
      void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });
    });
}

export function startImportInspect(environmentId: string, queryClient: QueryClient): void {
  const record = job.record;
  if (!record || isImportRunning(job.phase)) return;
  set({ phase: 'inspecting', environmentId, error: '' });
  void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });

  void api
    .post<VMImport>(`/vms/imports/${record.id}/inspect`, { environmentId })
    .then((response) => set({ phase: 'inspected', record: assertSuccess(response) }))
    .catch((error: Error) => set({ phase: 'failed', error: error.message }))
    .finally(() => {
      void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });
    });
}

export function startImportConvert(input: ImportConvertInput, queryClient: QueryClient): void {
  const record = job.record;
  if (!record || isImportRunning(job.phase)) return;
  set({ phase: 'converting', environmentId: input.environmentId, error: '' });
  void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });

  void api
    .post<VM>(`/vms/imports/${record.id}/convert`, input)
    .then((response) => {
      const vm = assertSuccess(response);
      set({ phase: 'done', vmId: vm?.id ?? '' });
    })
    .catch((error: Error) => set({ phase: 'failed', error: error.message }))
    .finally(() => {
      void queryClient.invalidateQueries({ queryKey: ['vms', 'imports'] });
      void queryClient.invalidateQueries({ queryKey: ['vms', 'list'] });
    });
}

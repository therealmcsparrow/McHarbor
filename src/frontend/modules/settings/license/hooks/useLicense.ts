// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import type {
  LicenseActivateInput,
  LicenseDetail,
  LicenseNoticeKind,
  LicenseStatus,
  LicenseUsageSample,
} from '../types';
import { assertSuccess } from '@resources/utils/api-mutation';

const STATUS_KEY = ['license', 'status'] as const;
const DETAIL_KEY = ['license', 'detail'] as const;

/**
 * The banner's data source. Polled rather than fetched once, because the
 * cadence that decides whether a notice is due lives on the server: a tab
 * left open overnight has to learn that tomorrow's showing came due
 * without the user reloading anything.
 */
export function useLicenseStatus() {
  return useQuery({
    queryKey: STATUS_KEY,
    queryFn: () => api.get<LicenseStatus>('/license/status').then(assertSuccess),
    staleTime: 60_000,
    refetchInterval: 5 * 60_000,
  });
}

/** The licence record and this install's seat identity. */
export function useLicenseDetail(enabled = true) {
  return useQuery({
    queryKey: DETAIL_KEY,
    queryFn: () => api.get<LicenseDetail>('/license').then(assertSuccess),
    enabled,
  });
}

/** Recorded usage samples, newest first. */
export function useLicenseUsage(page: number, perPage: number, enabled = true) {
  return useQuery({
    queryKey: ['license', 'usage', page, perPage],
    queryFn: () =>
      api
        .get<PaginatedData<LicenseUsageSample>>('/license/usage', {
          page: String(page),
          per_page: String(perPage),
        })
        .then(assertSuccess),
    enabled,
  });
}

/**
 * Claims a seat with the key the operator pasted. Safe to repeat: the
 * licence server refreshes the details for the same instance id and
 * consumes no second seat, which is what makes re-pasting a valid
 * recovery for an install whose seat was released.
 */
export function useActivateLicense() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: LicenseActivateInput) =>
      api.post<LicenseDetail>('/license/activate', input).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['license'] });
    },
  });
}

export function useReleaseLicense() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<null>('/license/deactivate').then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['license'] });
    },
  });
}

export function useRefreshLicense() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => api.post<null>('/license/refresh').then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['license'] });
    },
  });
}

/**
 * Records that a banner was actually shown.
 *
 * Deliberately quiet: no toast, no query invalidation, no error surfaced.
 * A failed acknowledgement means the banner shows again sooner than it
 * should, which is a far better failure than interrupting someone to tell
 * them a notice about a notice did not save.
 */
export function useAcknowledgeNotice() {
  return useMutation({
    mutationFn: (kind: LicenseNoticeKind) => api.post<null>('/license/notice-ack', { kind }),
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import type { APIKeyGrantInput } from './useAPIKeys';

export type DeviceRequest = {
  userCode: string;
  host: string;
  osUser: string;
  expiresAt: string;
  createdAt: string;
};

export type DeviceApproval = APIKeyGrantInput & {
  name: string;
  expiresAt?: string;
};

const ALPHABET = 'BCDFGHJKLMNPQRSTVWXZ';

export function normalizeUserCode(raw: string): string {
  const letters = raw
    .toUpperCase()
    .split('')
    .filter((c) => ALPHABET.includes(c))
    .join('');
  return letters.length === 8 ? `${letters.slice(0, 4)}-${letters.slice(4)}` : letters;
}

export function isCompleteUserCode(code: string): boolean {
  return /^[A-Z]{4}-[A-Z]{4}$/.test(code);
}

export function useDeviceRequest(code: string, enabled: boolean) {
  return useQuery({
    queryKey: ['device-request', code],
    queryFn: () => api.get<DeviceRequest>(`/device/requests/${encodeURIComponent(code)}`).then(assertSuccess),
    enabled: enabled && isCompleteUserCode(code),
    retry: false,
  });
}

export function useApproveDevice() {
  return useMutation({
    mutationFn: ({ code, body }: { code: string; body: DeviceApproval }) =>
      api.post(`/device/requests/${encodeURIComponent(code)}/approve`, body).then(assertSuccess),
  });
}

export function useDenyDevice() {
  return useMutation({
    mutationFn: (code: string) => api.post(`/device/requests/${encodeURIComponent(code)}/deny`).then(assertSuccess),
  });
}

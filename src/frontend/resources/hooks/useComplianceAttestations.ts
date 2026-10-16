// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export const ACCESS_REVIEW_FRAMEWORK = 'access-review';

export type ComplianceAttestation = {
  id: string;
  framework: string;
  runId: string;
  scope: string;
  attestedBy: string;
  attestedByName: string;
  attestedAt: string;
  note: string;
};

export type AttestationInput = {
  framework: string;
  runId?: string;
  scope?: string;
  note: string;
};

export function useLatestAttestation(framework: string, enabled: boolean) {
  return useQuery({
    queryKey: ['compliance', 'attestations', framework, 'latest'],
    enabled: enabled && framework !== '',
    queryFn: () =>
      api
        .get<PaginatedData<ComplianceAttestation>>('/compliance/attestations', { framework, per_page: '1' })
        .then(assertSuccess)
        .then((page) => page.items[0] ?? null),
    staleTime: 30_000,
  });
}

export function useCreateAttestation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AttestationInput) =>
      api.post<ComplianceAttestation>('/compliance/attestations', input).then(assertSuccess),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['compliance', 'attestations'] });
    },
  });
}

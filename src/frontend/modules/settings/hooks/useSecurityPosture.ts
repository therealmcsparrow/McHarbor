// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type PostureSeverity = 'critical' | 'warning';

export type PostureFinding = {
  envVar: string;
  severity: PostureSeverity;
  message: string;
};

export type SecurityPosture = {
  findings: PostureFinding[];
};

export function useSecurityPosture(enabled: boolean) {
  return useQuery({
    queryKey: ['settings', 'security-posture'],
    queryFn: () => api.get<SecurityPosture>('/settings/security-posture').then(assertSuccess),
    enabled,
    staleTime: 60_000,
  });
}

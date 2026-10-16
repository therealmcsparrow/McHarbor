// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { usePermission } from '@core/auth/usePermission';
import { PERM } from '@core/auth/permissionKeys';
import { RUNBOOKS_KEY } from '@resources/hooks/useRunbooks';
import type { AutomationInput, Runbook, ServicePrincipal } from '@resources/types/runbooks';

export function useServicePrincipals() {
  const allowed = usePermission(PERM.RUNBOOKS_AUTOMATE);
  return useQuery({
    queryKey: [...RUNBOOKS_KEY, 'principals'],
    enabled: allowed,
    queryFn: () => api.get<ServicePrincipal[]>('/runbooks/principals').then(assertSuccess),
  });
}

export function useSetAutomation(runbookId: string) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (input: AutomationInput) =>
      api.put<Runbook>(`/runbooks/${encodeURIComponent(runbookId)}/automation`, input).then(assertSuccess),
    onSuccess: () => void client.invalidateQueries({ queryKey: RUNBOOKS_KEY }),
  });
}

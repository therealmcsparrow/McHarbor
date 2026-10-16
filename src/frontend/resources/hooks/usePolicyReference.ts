// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type ReferenceAttribute = { path: string; type: string; description: string };

export type ReferenceResourceType = {
  type: string;
  label: string;
  category: string;
  source: string;
  resolvable: boolean;
  attributes: ReferenceAttribute[];
};

export type ReferenceHelper = { name: string; signature: string; description: string; example: string };

export type PolicyReference = {
  resourceTypes: ReferenceResourceType[];
  variables: ReferenceAttribute[];
  helpers: ReferenceHelper[];
};

export function usePolicyReference(enabled = true) {
  return useQuery({
    queryKey: ['rbac', 'policy-reference'],
    enabled,
    queryFn: () => api.get<PolicyReference>('/rbac/policy-reference').then(assertSuccess),
    staleTime: 60_000,
  });
}

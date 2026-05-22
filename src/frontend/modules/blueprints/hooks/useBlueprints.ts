// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type Blueprint = {
  id: string;
  name: string;
  description: string;
  category: string;
  icon: string;
  composeYaml: string;
  envVars: string;
  version: string;
  createdAt: string;
  updatedAt: string;
};

export type BlueprintInput = {
  name: string;
  description: string;
  category: string;
  icon: string;
  composeYaml: string;
  envVars: string;
  version: string;
};

export function useBlueprints() {
  return useQuery({
    queryKey: ['blueprints'],
    queryFn: () =>
      api.get<PaginatedData<Blueprint>>('/blueprints', { per_page: '500' }).then((r) => r.data?.items ?? []),
  });
}

export function useCreateBlueprint(successMsg: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: BlueprintInput) => api.post('/blueprints', input).then(assertSuccess),
    meta: { success: successMsg },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['blueprints'] }),
  });
}

export function useUpdateBlueprint(successMsg: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: Partial<BlueprintInput> }) =>
      api.put(`/blueprints/${id}`, input).then(assertSuccess),
    meta: { success: successMsg },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['blueprints'] }),
  });
}

export function useDeleteBlueprint(successMsg: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.del(`/blueprints/${id}`).then(assertSuccess),
    meta: { success: successMsg },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['blueprints'] }),
  });
}

export function useDeployBlueprint(successMsg: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, stackName }: { id: string; stackName: string }) =>
      api.post(`/blueprints/${id}/deploy`, { stackName }).then(assertSuccess),
    meta: { success: successMsg },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['stacks'] }),
  });
}

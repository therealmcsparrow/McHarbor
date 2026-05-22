// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';

export type WidgetCatalogDefinition = {
  key: string;
  label: string;
  category: string;
  description: string;
  icon: string;
  source: string;
  component: string;
  enabled: boolean;
  defaultSize: { w: number; h: number };
  minSize: { w: number; h: number };
  translations?: Record<string, Record<string, unknown>>;
};

export function useWidgetCatalogDefinitions() {
  return useQuery({
    queryKey: ['widgets', 'definitions'],
    queryFn: () => api.get<WidgetCatalogDefinition[]>('/widgets/definitions').then(assertSuccess),
    staleTime: 5 * 60_000,
  });
}

export function useUpdateWidgetCatalogEnabled() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) =>
      api.put<{ key: string; enabled: boolean }>(`/widgets/${key}`, { enabled }).then(assertSuccess),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['widgets', 'definitions'] });
    },
  });
}

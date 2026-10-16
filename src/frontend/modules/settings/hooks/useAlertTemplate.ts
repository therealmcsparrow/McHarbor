// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { AlertConditionConfig } from '@resources/hooks/useAlerts';

export type AlertTemplateVariable = { name: string; description: string; sample: string };
export type AlertTemplateContextKey = { key: string; description: string; learned: boolean };

export type AlertTemplateReference = {
  variables: AlertTemplateVariable[];
  context: AlertTemplateContextKey[];
};

export type AlertTemplatePreviewInput = {
  template: string;
  name: string;
  severity: string;
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  state: 'firing' | 'resolved';
};

export type AlertTemplatePreview = { message: string; unknown: string[] };

export function useAlertTemplateReference(sourceType: string, conditionKey: string) {
  return useQuery({
    queryKey: ['alerts', 'template-reference', sourceType, conditionKey],
    queryFn: () =>
      api
        .get<AlertTemplateReference>(
          `/alerts/template-reference?source=${encodeURIComponent(sourceType)}&condition=${encodeURIComponent(conditionKey)}`,
        )
        .then((r) => r.data ?? { variables: [], context: [] }),
    staleTime: 60_000,
  });
}

function useDebounced<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const id = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(id);
  }, [value, delay]);
  return debounced;
}

export function useAlertTemplatePreview(input: AlertTemplatePreviewInput) {
  const debounced = useDebounced(input, 300);
  return useQuery({
    queryKey: ['alerts', 'template-preview', debounced],
    queryFn: () =>
      api
        .post<AlertTemplatePreview>('/alerts/template-preview', debounced)
        .then((r) => r.data ?? { message: '', unknown: [] }),
    placeholderData: keepPreviousData,
  });
}

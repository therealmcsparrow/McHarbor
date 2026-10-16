// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '@core/api/client';
import type { Incident } from '@resources/hooks/useIncidents';

export type IncidentSource = {
  source: string;
  subjectTypes: { type: string; label: string; signals: { key: string; label: string; strength: number }[] }[];
};

export function useIncidentSources() {
  return useQuery({
    queryKey: ['incidents', 'sources'],
    queryFn: () => api.get<IncidentSource[]>('/incidents/sources').then((r) => r.data ?? []),
    staleTime: 60_000,
  });
}

export function useIncidentLabels() {
  const { t } = useTranslation('incidents');
  const sources = useIncidentSources();
  const index = useMemo(() => {
    const types = new Map<string, string>();
    const signals = new Map<string, string>();
    for (const src of sources.data ?? []) {
      for (const st of src.subjectTypes) {
        types.set(`${src.source}/${st.type}`, st.label);
        for (const sig of st.signals) signals.set(`${src.source}/${st.type}/${sig.key}`, sig.label);
      }
    }
    return { types, signals };
  }, [sources.data]);

  const signal = useCallback(
    (in_: Pick<Incident, 'source' | 'subjectType' | 'signal'>) =>
      in_.source
        ? (index.signals.get(`${in_.source}/${in_.subjectType}/${in_.signal}`) ?? in_.signal)
        : t(`signal.${in_.signal}`),
    [index, t],
  );
  const subjectType = useCallback(
    (in_: Pick<Incident, 'source' | 'subjectType'>) =>
      in_.source
        ? (index.types.get(`${in_.source}/${in_.subjectType}`) ?? in_.subjectType)
        : t('subjectType.container'),
    [index, t],
  );
  return { signal, subjectType };
}

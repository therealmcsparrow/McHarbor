// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';

/**
 * How a condition is measured. Drives which controls the builder
 * renders and how the backend's state machine reads an observation.
 */
export type AlertConditionKind = 'threshold' | 'duration' | 'event' | 'count_in_window';

/** What a rule's target pattern is matched against. */
export type AlertTargetKind = 'container' | 'environment' | 'none';

export type AlertConditionField = {
  key: string;
  type: string;
  label: string;
  required?: boolean;
  help?: string;
  placeholder?: string;
};

export type AlertCondition = {
  key: string;
  displayName: string;
  description?: string;
  kind: AlertConditionKind;
  unit?: string;
  operators: string[];
  defaultOperator?: string;
  defaultThreshold?: number;
  supportsFor: boolean;
  targetKind: AlertTargetKind;
  fields?: AlertConditionField[];
  /**
   * Set for built-in conditions only. Plugin conditions ship their own
   * localised displayName, so there is nothing for the host to look up.
   */
  i18nKey?: string;
};

export type AlertConditionSource = {
  sourceType: string;
  displayName: string;
  description?: string;
  slug?: string;
  icon?: string;
  builtIn: boolean;
  conditions: AlertCondition[];
};

export type AlertConditionCatalog = {
  sources: AlertConditionSource[];
};

const EMPTY_CATALOG: AlertConditionCatalog = { sources: [] };

/**
 * Everything that can be alerted on, built-in and plugin-contributed.
 *
 * The list is deliberately not hardcoded in the frontend: a plugin that
 * registers alert conditions must appear here without a frontend
 * change, which is the whole point of the backend's source registry.
 */
export function useAlertConditions() {
  return useQuery({
    queryKey: ['alert-conditions'],
    queryFn: () =>
      api.get<AlertConditionCatalog>('/alerts/conditions').then((response) => response.data ?? EMPTY_CATALOG),
    staleTime: 5 * 60 * 1000,
  });
}

/** Finds one condition declaration in the catalog. */
export function findCondition(
  catalog: AlertConditionCatalog | undefined,
  sourceType: string,
  conditionKey: string,
): { source: AlertConditionSource; condition: AlertCondition } | undefined {
  for (const source of catalog?.sources ?? []) {
    if (source.sourceType !== sourceType) continue;
    for (const condition of source.conditions) {
      if (condition.key === conditionKey) {
        return { source, condition };
      }
    }
  }
  return undefined;
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { FilterFacet } from '@resources/components/FilterChips';
import type { Secret } from '../types';

export const SECRET_GROUP_ALL = '__all__';
export const SECRET_GROUP_UNGROUPED = '__ungrouped__';

type SecretGroups = {
  names: string[];
  kinds: string[];
  facets: FilterFacet<string>[];
  filter: (rows: Secret[], selected: string) => Secret[];
};

export function useSecretGroups(secrets: Secret[]): SecretGroups {
  const { t } = useTranslation('secrets');

  return useMemo(() => {
    const counts = new Map<string, number>();
    let ungrouped = 0;
    for (const secret of secrets) {
      const group = secret.group?.trim();
      if (!group) {
        ungrouped += 1;
        continue;
      }
      counts.set(group, (counts.get(group) ?? 0) + 1);
    }

    const names = [...counts.keys()].sort((a, b) => a.localeCompare(b));
    const kinds = [...new Set(secrets.map((secret) => secret.kind))].sort((a, b) => a.localeCompare(b));

    const facets: FilterFacet<string>[] = [
      { id: SECRET_GROUP_ALL, label: t('groups.all'), count: secrets.length },
      ...names.map((itemName) => ({ id: itemName, label: itemName, count: counts.get(itemName) ?? 0 })),
    ];
    if (ungrouped > 0) {
      facets.push({
        id: SECRET_GROUP_UNGROUPED,
        label: t('groups.ungrouped'),
        count: ungrouped,
      });
    }

    const filter = (rows: Secret[], selected: string) => {
      if (selected === SECRET_GROUP_ALL) return rows;
      if (selected === SECRET_GROUP_UNGROUPED) return rows.filter((row) => !row.group?.trim());
      return rows.filter((row) => row.group?.trim() === selected);
    };

    return { names, kinds, facets, filter };
  }, [secrets, t]);
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import type { RoleFilter } from './useClusterInstanceFilters';

const ROLES = ['active', 'standby', 'peer'] as const;

export function ClusterInstancesFilters({
  value,
  onChange,
}: {
  value: RoleFilter;
  onChange: (value: RoleFilter) => void;
}) {
  const { t } = useTranslation('enterprise');
  return (
    <Select
      value={value}
      onChange={(next) => onChange(ROLES.find((role) => role === next) ?? '')}
      ariaLabel={t('ha.columns.role')}
      searchable={false}
      className="w-40"
      options={[
        { value: '', label: t('ha.filter.allRoles') },
        ...ROLES.map((role) => ({ value: role, label: t(`ha.role.${role}`) })),
      ]}
    />
  );
}

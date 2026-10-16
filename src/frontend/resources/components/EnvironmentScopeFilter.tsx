// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import { useEnvironmentStore } from '@resources/stores/environment';

type EnvironmentScopeFilterProps = {
  value: string;
  onChange: (envId: string) => void;
  className?: string;
};

export function EnvironmentScopeFilter({ value, onChange, className = 'w-52' }: EnvironmentScopeFilterProps) {
  const { t } = useTranslation('common');
  const environments = useEnvironmentStore((s) => s.environments);

  return (
    <Select
      value={value}
      onChange={onChange}
      options={[
        { value: '', label: t('header.allEnvironments') },
        ...environments.map((env) => ({ value: env.id, label: env.name })),
      ]}
      ariaLabel={t('activity.filters.environment')}
      className={className}
    />
  );
}

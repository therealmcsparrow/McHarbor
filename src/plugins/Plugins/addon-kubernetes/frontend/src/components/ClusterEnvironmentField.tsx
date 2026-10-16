// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { FormField } from '@resources/components/ui/FormField';
import { Select } from '@resources/components/ui/Select';
import { useHostEnvironments } from '../hooks/useHostEnvironments';

const NONE = '';

/** Which McHarbor environment a cluster runs on; the link puts it on that environment's card. */
export function ClusterEnvironmentField({
  value,
  disabled,
  onChange,
}: {
  value: string;
  disabled: boolean;
  onChange: (environmentId: string) => void;
}) {
  const { t } = useTranslation('kubernetes');
  const { data: environments = [] } = useHostEnvironments();
  // A linked environment the reader cannot list still shows as itself, not as "none".
  const known = value === NONE || environments.some((env) => env.id === value);

  return (
    <FormField
      label={t('clusters.form.environment')}
      htmlFor="k8s-cluster-environment"
      description={t('clusters.form.environmentHelp')}
    >
      <Select
        value={value}
        ariaLabel={t('clusters.form.environment')}
        disabled={disabled}
        options={[
          { value: NONE, label: t('clusters.form.environmentNone') },
          ...environments.map((env) => ({ value: env.id, label: env.name })),
          ...(known ? [] : [{ value, label: value }]),
        ]}
        onChange={onChange}
      />
    </FormField>
  );
}

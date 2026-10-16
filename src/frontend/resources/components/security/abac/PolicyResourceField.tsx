// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import type { ReferenceResourceType } from '@resources/hooks/usePolicyReference';

type Props = {
  value: string;
  types: ReferenceResourceType[];
  onChange: (value: string) => void;
};

export function PolicyResourceField({ value, types, onChange }: Props) {
  const { t } = useTranslation('abac');
  const label = (rt: ReferenceResourceType) =>
    rt.source === 'host'
      ? t(`reference.types.${rt.type}`)
      : t('editor.pluginSource', { label: rt.label, source: rt.source });
  const known = types.some((rt) => rt.type === value);
  const options = [
    { value: '', label: t('editor.anyResource') },
    ...types.map((rt) => ({ value: rt.type, label: label(rt) })),
    ...(value && !known ? [{ value, label: value }] : []),
  ];
  return (
    <div className="space-y-1">
      <Label htmlFor="abac-resource">{t('editor.resourceType')}</Label>
      <Select value={value} onChange={onChange} options={options} ariaLabel={t('editor.resourceType')} />
    </div>
  );
}

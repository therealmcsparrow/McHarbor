// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import type { PolicyInput } from '@resources/hooks/usePolicies';
import type { ReferenceResourceType } from '@resources/hooks/usePolicyReference';
import { PolicyResourceField } from '@resources/components/security/abac/PolicyResourceField';

type Props = {
  form: PolicyInput;
  types: ReferenceResourceType[];
  onChange: (next: PolicyInput) => void;
};

export function PolicyBasicsFields({ form, types, onChange }: Props) {
  const { t } = useTranslation('abac');
  return (
    <>
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        <div className="space-y-1">
          <Label htmlFor="abac-name">{t('editor.name')}</Label>
          <Input
            id="abac-name"
            value={form.name}
            onChange={(e) => onChange({ ...form, name: e.target.value })}
            placeholder={t('editor.namePlaceholder')}
          />
        </div>
        <PolicyResourceField
          value={form.resourceType}
          types={types}
          onChange={(resourceType) => onChange({ ...form, resourceType })}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="abac-desc">{t('editor.description')}</Label>
        <Input
          id="abac-desc"
          value={form.description}
          onChange={(e) => onChange({ ...form, description: e.target.value })}
          placeholder={t('editor.descriptionPlaceholder')}
        />
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div className="space-y-1">
          <Label htmlFor="abac-effect">{t('editor.effect')}</Label>
          <Select
            value={form.effect}
            onChange={(v) => onChange({ ...form, effect: v as 'allow' | 'deny' })}
            options={[
              { value: 'allow', label: t('editor.effectAllow') },
              { value: 'deny', label: t('editor.effectDeny') },
            ]}
            ariaLabel={t('editor.effect')}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="abac-priority">{t('editor.priority')}</Label>
          <NumberInput
            id="abac-priority"
            value={form.priority}
            onChange={(priority) => onChange({ ...form, priority })}
          />
        </div>
        <div className="flex items-end gap-2 pb-1">
          <Switch
            id="abac-enabled"
            checked={form.enabled}
            onCheckedChange={(enabled) => onChange({ ...form, enabled })}
          />
          <Label htmlFor="abac-enabled" className="cursor-pointer">
            {t('editor.enabled')}
          </Label>
        </div>
      </div>
    </>
  );
}

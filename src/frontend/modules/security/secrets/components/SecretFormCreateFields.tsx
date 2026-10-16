// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { FormField } from '@resources/components/ui/FormField';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { SecretCreatableSelect } from './SecretCreatableSelect';
import { kindLabel } from '@resources/utils/secret-kind';
import { normalizeKind } from '../kind-normalize';
import { SECRET_ELEVATED_SCOPES, SECRET_KINDS, SECRET_SCOPES, type SecretKind, type SecretScope } from '../types';

type SecretFormCreateFieldsProps = {
  kind: SecretKind;
  onKindChange: (kind: SecretKind) => void;
  scope: SecretScope;
  onScopeChange: (scope: SecretScope) => void;
  stackName: string;
  onStackNameChange: (stackName: string) => void;
  group: string;
  onGroupChange: (group: string) => void;
  groupSuggestions: string[];
  kindSuggestions: string[];
  description: string;
  onDescriptionChange: (description: string) => void;
  autoApply: boolean;
  onAutoApplyChange: (autoApply: boolean) => void;
  canManageGlobal: boolean;
};

export function SecretFormCreateFields({
  kind,
  onKindChange,
  scope,
  onScopeChange,
  stackName,
  onStackNameChange,
  group,
  onGroupChange,
  groupSuggestions,
  kindSuggestions,
  description,
  onDescriptionChange,
  autoApply,
  onAutoApplyChange,
  canManageGlobal,
}: SecretFormCreateFieldsProps) {
  const { t } = useTranslation('secrets');

  const kindOptions = useMemo(() => {
    const values = [...new Set([...SECRET_KINDS, ...kindSuggestions])];
    return values.map((value) => ({ value, label: kindLabel(t, value) }));
  }, [kindSuggestions, t]);

  return (
    <>
      <FormField label={t('fields.kind')} description={t('fields.kindHint')}>
        <SecretCreatableSelect
          value={kind}
          onChange={(next) => onKindChange(next as SecretKind)}
          options={kindOptions}
          ariaLabel={t('fields.kind')}
          placeholder={t('fields.kindPlaceholder')}
          searchPlaceholder={t('kinds.searchPlaceholder')}
          createLabel={(entryName) => t('kinds.create', { name: entryName })}
          emptyLabel={t('kinds.none')}
          normalize={normalizeKind}
          maxLength={32}
        />
      </FormField>

      <FormField label={t('fields.scope')} description={t('fields.scopeHint')}>
        {/*
          The global option is disabled with an explanation rather than
          hidden: a user who cannot create a global secret should still
          learn that the capability exists and which permission unlocks
          it, instead of finding out from a 403.
        */}
        <div className="space-y-1.5">
          <Select
            value={scope}
            onChange={(next) => onScopeChange(next as SecretScope)}
            options={SECRET_SCOPES.filter((option) => canManageGlobal || !SECRET_ELEVATED_SCOPES.includes(option)).map(
              (option) => ({ value: option, label: t(`scope.${option}`) }),
            )}
            ariaLabel={t('fields.scope')}
          />
          {!canManageGlobal && (
            <p className="text-xs text-muted-foreground">
              {t('fields.globalDenied', { permission: PERM.SECRETS_MANAGE_GLOBAL })}
            </p>
          )}
        </div>
      </FormField>

      {scope === 'stack' && (
        <FormField label={t('fields.stackName')}>
          <Input
            variant="outline"
            value={stackName}
            onChange={(eventItem) => onStackNameChange(eventItem.target.value)}
            placeholder={t('fields.stackNamePlaceholder')}
          />
        </FormField>
      )}

      <FormField label={t('fields.group')} description={t('fields.groupHint')}>
        <SecretCreatableSelect
          value={group}
          onChange={onGroupChange}
          options={groupSuggestions.map((nameValue) => ({ value: nameValue, label: nameValue }))}
          ariaLabel={t('fields.group')}
          placeholder={t('groups.ungrouped')}
          searchPlaceholder={t('groups.searchPlaceholder')}
          createLabel={(itemName) => t('groups.create', { name: itemName })}
          emptyLabel={t('groups.none')}
          clearLabel={t('groups.clear')}
        />
      </FormField>

      <FormField label={t('fields.description')}>
        <Textarea
          value={description}
          onChange={(evt) => onDescriptionChange(evt.target.value)}
          rows={2}
          placeholder={t('fields.descriptionPlaceholder')}
        />
      </FormField>

      <FormField label={t('fields.autoApply')} description={t('fields.autoApplyHint')}>
        <Switch checked={autoApply} onCheckedChange={onAutoApplyChange} />
      </FormField>
    </>
  );
}

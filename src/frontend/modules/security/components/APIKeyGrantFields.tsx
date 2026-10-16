// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { Label } from '@resources/components/ui/Label';
import { MultiSelect, type MultiSelectOption } from '@resources/components/ui/MultiSelect';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { PermissionEditor, type PermissionSelection } from '@resources/components/security/PermissionEditor';
import type { APIKeyGrantOptions } from '../hooks/useAPIKeys';

type APIKeyGrantFieldsProps = {
  options: APIKeyGrantOptions | undefined;
  loading: boolean;
  roleIds: string[];
  environmentIds: string[];
  selection: PermissionSelection;
  scopeCount: number;
  onRolesChange: (next: string[]) => void;
  onEnvironmentsChange: (next: string[]) => void;
  onSelectionChange: (next: PermissionSelection) => void;
};

export function APIKeyGrantFields({
  options,
  loading,
  roleIds,
  environmentIds,
  selection,
  scopeCount,
  onRolesChange,
  onEnvironmentsChange,
  onSelectionChange,
}: APIKeyGrantFieldsProps) {
  const { t } = useTranslation('security');

  // A role the caller does not fully hold stays visible but unselectable,
  // carrying the reason — the same rule the backend enforces on save.
  const roleOptions = useMemo<MultiSelectOption[]>(
    () =>
      (options?.roles ?? []).map((role) => ({
        value: role.id,
        label: role.name,
        description: role.description || undefined,
        disabled: !role.grantable,
        disabledReason: role.grantable ? undefined : t('apiKeys.cannotGrant'),
      })),
    [options, t],
  );

  const environmentOptions = useMemo<MultiSelectOption[]>(
    () =>
      (options?.environments ?? []).map((env) => ({
        value: env.id,
        label: env.name,
      })),
    [options],
  );

  if (loading || !options) {
    return (
      <div className="space-y-4">
        <div className="space-y-2">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-9 w-full" />
        </div>
        <div className="space-y-2">
          <Skeleton className="h-4 w-28" />
          <Skeleton className="h-9 w-full" />
        </div>
        <Skeleton className="h-12 w-full" />
      </div>
    );
  }

  const handleSelection = (next: PermissionSelection) => {
    // The wildcard is only honoured when the caller holds it; the backend
    // rejects it otherwise, so mirror that here.
    if (next.wildcard && !options.wildcard) {
      onSelectionChange({ ...next, wildcard: false });
      return;
    }
    onSelectionChange(next);
  };

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t('apiKeys.roles')}</Label>
        <MultiSelect
          value={roleIds}
          onChange={onRolesChange}
          options={roleOptions}
          placeholder={t('apiKeys.selectRoles')}
          ariaLabel={t('apiKeys.roles')}
          className="w-full"
        />
      </div>

      <div className="space-y-2">
        <Label>{t('apiKeys.environments')}</Label>
        <MultiSelect
          value={environmentIds}
          onChange={onEnvironmentsChange}
          options={environmentOptions}
          emptyLabel={t('apiKeys.allEnvironments')}
          ariaLabel={t('apiKeys.environments')}
          className="w-full"
        />
        <p className="text-xs text-muted-foreground">{t('apiKeys.environmentsHint')}</p>
      </div>

      <Disclosure
        title={t('apiKeys.additionalPermissions')}
        description={t('apiKeys.additionalPermissionsHint')}
        trailing={
          <span className="text-xs text-muted-foreground">
            {selection.wildcard ? t('apiKeys.wildcardGrant') : t('apiKeys.grantsCount', { count: scopeCount })}
          </span>
        }
      >
        {/* The two pickers above collapsed to one row each, so the catalog
            gets the height it actually needs to be browsable. */}
        <PermissionEditor value={selection} onChange={handleSelection} listClassName="max-h-[55vh] min-h-[20rem]" />
      </Disclosure>
    </div>
  );
}

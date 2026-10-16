// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandAzure, IconBrandGoogle, IconShieldLock } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { brandText } from '@resources/theme/brand-colors';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { IdentityAdapterSpec } from '@resources/hooks/useIdentityAdapters';
import { textColumn, boolColumn, listColumn, dateColumn } from '@resources/layout/table-page/optional-columns';
import type { IdentityProvider as IdentityProviderValue } from '../hooks/useIdentityProviders';
import { IdentityProviderActions } from './IdentityProviderActions';

/** The provider's own mark, so the eye lands on the brand before the name.
 *
 *  Fallback only. A provider's real mark is the owning plugin's
 *  icons/plugin.svg, resolved from the adapter catalog so the row
 *  matches the tile the operator picked in CreateProviderDialog. This
 *  map covers the gap when the plugin is uninstalled or the catalog
 *  hasn't loaded — provider rows outlive their plugin in the DB. */
function providerFallbackIcon(providerType: IdentityProviderValue['providerType']) {
  if (providerType === 'entra_id') {
    return { Icon: IconBrandAzure, className: brandText('microsoft') };
  }
  if (providerType === 'google') {
    return { Icon: IconBrandGoogle, className: brandText('google') };
  }
  return { Icon: IconShieldLock, className: 'text-info' };
}

/** Human label for a provider type, also used as a search field. */
export function useProviderTypeLabel() {
  const { t } = useTranslation('security');
  return useMemo(
    () => (providerType: IdentityProviderValue['providerType']) => {
      if (providerType === 'entra_id') return t('identity.entraId');
      if (providerType === 'google') return t('identity.google');
      if (providerType === 'generic_oidc') return t('identity.genericOidc');
      return t('identity.saml2');
    },
    [t],
  );
}

type ColumnsOptions = {
  adapters: Record<string, IdentityAdapterSpec> | undefined;
  typeLabel: (providerType: IdentityProviderValue['providerType']) => string;
  onEdit: (provider: IdentityProviderValue) => void;
  groupNameById: Map<string, string>;
};

/** Columns for the identity-provider table. */
export function useIdentityProviderColumns({ adapters, typeLabel, onEdit, groupNameById }: ColumnsOptions) {
  const { t } = useTranslation('security');
  const group = t('columns.groupProvider');
  const access = t('columns.groupAccess');

  return useMemo<ColumnDef<IdentityProviderValue, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (provider) => provider.name,
        header: t('identity.provider'),
        cell: ({ row }) => {
          const spec = adapters?.[row.original.providerType];
          const { Icon, className } = providerFallbackIcon(row.original.providerType);
          return (
            <div className="flex items-center gap-3">
              {spec?.Slug && spec.Icon ? (
                <PluginAssetImage slug={spec.Slug} icon={spec.Icon} className="size-6 shrink-0 object-contain" />
              ) : (
                <Icon className={`size-6 shrink-0 ${className}`} />
              )}
              <span className="truncate font-medium">{row.original.name}</span>
            </div>
          );
        },
      },
      {
        id: 'type',
        accessorFn: (provider) => typeLabel(provider.providerType),
        header: t('identity.type'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      {
        id: 'status',
        accessorFn: (provider) => (provider.enabled ? t('identity.enabled') : t('identity.disabled')),
        header: t('identity.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={row.original.enabled ? 'default' : 'secondary'}>
            {row.original.enabled ? t('identity.enabled') : t('identity.disabled')}
          </Badge>
        ),
      },
      textColumn<IdentityProviderValue>('providerId', t('columns.id'), (r) => r.id, { group }),
      textColumn<IdentityProviderValue>('tenantId', t('columns.tenantId'), (r) => r.tenantId, { group }),
      textColumn<IdentityProviderValue>('domain', t('columns.domain'), (r) => r.domain, { group }),
      textColumn<IdentityProviderValue>('issuerUrl', t('columns.issuerUrl'), (r) => r.issuerUrl, {
        group,
        width: 'xl',
      }),
      textColumn<IdentityProviderValue>('metadataUrl', t('columns.metadataUrl'), (r) => r.metadataUrl, {
        group,
        width: 'xl',
      }),
      textColumn<IdentityProviderValue>('entityId', t('columns.entityId'), (r) => r.entityId, { group, width: 'lg' }),
      textColumn<IdentityProviderValue>('scopes', t('columns.scopes'), (r) => r.scopes, { group }),
      boolColumn<IdentityProviderValue>('autoProvision', t('columns.autoProvision'), (r) => r.autoProvision, { group }),
      boolColumn<IdentityProviderValue>(
        'groupMappingEnabled',
        t('columns.groupMappingEnabled'),
        (r) => r.groupMappingEnabled,
        {
          group,
        },
      ),
      boolColumn<IdentityProviderValue>('autoImportGroups', t('columns.autoImportGroups'), (r) => r.autoImportGroups, {
        group,
      }),
      listColumn<IdentityProviderValue>(
        'mappedGroups',
        t('columns.mappedGroups'),
        (r) => (r.groupMappings ?? []).map((m) => groupNameById.get(m.mcharborGroupId) ?? m.mcharborGroupId),
        { group: access },
      ),
      textColumn<IdentityProviderValue>('defaultRole', t('columns.defaultRole'), (r) => r.defaultRoleName, {
        group: access,
      }),
      dateColumn<IdentityProviderValue>('createdAt', t('columns.createdAt'), (r) => r.createdAt, { group }),
      dateColumn<IdentityProviderValue>('updatedAt', t('columns.updatedAt'), (r) => r.updatedAt, { group }),
      {
        id: 'actions',
        header: t('identity.actions'),
        enableSorting: false,
        meta: { width: 'md', locked: true, noExport: true, label: t('identity.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => <IdentityProviderActions provider={row.original} onEdit={onEdit} />,
      },
    ],
    [adapters, t, typeLabel, onEdit, group, access, groupNameById],
  );
}

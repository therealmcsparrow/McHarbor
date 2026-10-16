// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconKeyOff, IconPencil, IconRefresh, IconTrash } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@resources/components/ui/Tooltip';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { timeAgo } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { APIKey } from '../hooks/useAPIKeys';
import { useAPIKeyOptionalColumns } from './apiKeyOptionalColumns';
import { APIKeyStatusCell } from './governance/APIKeyStatusCell';

type APIKeyColumnActions = {
  onView: (key: APIKey) => void;
  onEdit: (key: APIKey) => void;
  onRevoke: (key: APIKey) => void;
  onDelete: (key: APIKey) => void;
  onRotate: (key: APIKey) => void;
};

/**
 * Column definitions for the API keys table. Kept out of APIKeysTab so the
 * tab stays a thin orchestration component.
 */
export function useAPIKeyColumns({ onView, onEdit, onRevoke, onDelete, onRotate }: APIKeyColumnActions) {
  const { t } = useTranslation('security');
  const optionalColumns = useAPIKeyOptionalColumns();

  return useMemo<ColumnDef<APIKey, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (key) => key.name,
        header: t('apiKeys.name'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div>
            <div className="font-medium">{row.original.name}</div>
            <div className="text-xs text-muted-foreground">{row.original.username}</div>
          </div>
        ),
      },
      {
        id: 'key',
        accessorFn: (key) => key.keyPrefix,
        header: t('apiKeys.key'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => <code className="rounded bg-muted px-1.5 py-0.5 text-xs">{row.original.keyPrefix}</code>,
      },
      {
        id: 'roles',
        accessorFn: (key) => key.roles.map((role) => role.name).join(', '),
        header: t('apiKeys.roles'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.roles.length === 0 ? (
            <span className="text-xs text-muted-foreground">{t('apiKeys.noRoles')}</span>
          ) : (
            <div className="flex flex-wrap gap-1">
              {row.original.roles.map((role) => (
                <Badge key={role.id}>{role.name}</Badge>
              ))}
            </div>
          ),
      },
      {
        id: 'environments',
        accessorFn: (key) =>
          key.environmentIds.length === 0 ? t('apiKeys.allEnvironments') : String(key.environmentIds.length),
        header: t('apiKeys.environments'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
      },
      {
        id: 'scopes',
        accessorFn: (key) => (key.scopes.includes('*') ? t('apiKeys.wildcardGrant') : key.scopes.length),
        header: t('apiKeys.scopes'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const { scopes } = row.original;
          if (scopes.includes('*')) return <Badge variant="warning">{t('apiKeys.wildcardGrant')}</Badge>;
          if (scopes.length === 0) return <span className="text-xs text-muted-foreground">0</span>;
          return (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="text-sm">{t('apiKeys.grantsCount', { count: scopes.length })}</span>
                </TooltipTrigger>
                <TooltipContent>
                  <ul className="space-y-0.5 text-xs">
                    {scopes.map((scope) => (
                      <li key={scope}>{scope}</li>
                    ))}
                  </ul>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          );
        },
      },
      {
        id: 'expiresAt',
        accessorFn: (key) => (key.expiresAt ? timeAgo(key.expiresAt) : t('apiKeys.noExpiry')),
        header: t('apiKeys.expiresAt'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      {
        id: 'lastUsed',
        accessorFn: (key) => (key.lastUsedAt ? timeAgo(key.lastUsedAt) : t('apiKeys.never')),
        header: t('apiKeys.lastUsed'),
        meta: { width: 'md' } satisfies TableColumnMeta,
      },
      {
        id: 'status',
        // The server derives status now, so the table no longer recomputes
        // expiry from the timestamp. That local guess had no way to express
        // "deleted" and reported a tombstone as merely revoked.
        accessorFn: (key) => t(`apiKeys.${key.status}`),
        header: t('apiKeys.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => <APIKeyStatusCell apiKey={row.original} />,
      },
      ...optionalColumns,
      {
        id: 'actions',
        header: () => <span className="ml-auto">{t('labels.actions', { ns: 'common' })}</span>,
        enableSorting: false,
        meta: {
          width: 'sm',
          locked: true,
          noExport: true,
          label: t('labels.actions', { ns: 'common' }),
        } satisfies TableColumnMeta,
        // Every key state gets the detail action, deleted ones included:
        // a tombstone is exactly the row whose history someone needs.
        // The mutation actions then depend on state — an active key offers
        // edit and revoke, a revoked one offers permanent delete, and a
        // deleted one has nothing left to act on.
        cell: ({ row }) => (
          <div className="flex justify-end gap-1">
            <Button
              permission={[PERM.API_KEYS_VIEW, PERM.API_KEYS_MANAGE]}
              variant="ghost"
              size="icon-sm"
              onClick={() => onView(row.original)}
              aria-label={t('apiKeys.detail.viewAction')}
            >
              <IconEye className="size-3.5" />
            </Button>
            {row.original.status === 'deleted' ? null : row.original.isActive ? (
              <>
                <Button
                  permission={PERM.API_KEYS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onEdit(row.original)}
                  aria-label={t('apiKeys.editKey')}
                >
                  <IconPencil className="size-3.5" />
                </Button>
                <Button
                  permission={PERM.API_KEYS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onRotate(row.original)}
                  aria-label={t('governance.apiKeys.rotate')}
                >
                  <IconRefresh className="size-3.5" />
                </Button>
                <Button
                  permission={PERM.API_KEYS_MANAGE}
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => onRevoke(row.original)}
                  aria-label={t('apiKeys.revokeKey')}
                >
                  <IconKeyOff className="size-3.5" />
                </Button>
              </>
            ) : (
              <Button
                permission={PERM.API_KEYS_MANAGE}
                variant="ghost"
                size="icon-sm"
                onClick={() => onDelete(row.original)}
                aria-label={t('apiKeys.deleteKey')}
              >
                <IconTrash className="size-3.5" />
              </Button>
            )}
          </div>
        ),
      },
    ],
    [t, optionalColumns, onView, onEdit, onRevoke, onDelete, onRotate],
  );
}

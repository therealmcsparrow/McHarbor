// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ColumnDef } from '@resources/components/table-features';
import { IconShieldLock, IconEdit, IconTrash, IconPlayerPlay, IconPlayerPause, IconFlask } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { useTogglePolicy, type Policy } from '@resources/hooks/usePolicies';
import { PERM } from '@core/auth/permissionKeys';

type UsePolicyColumnsArgs = {
  toggle: ReturnType<typeof useTogglePolicy>;
  onTest: (policy: Policy) => void;
  onEdit: (policy: Policy) => void;
  onDelete: (policy: Policy) => void;
};

export function usePolicyColumns({ toggle, onTest, onEdit, onDelete }: UsePolicyColumnsArgs) {
  const { t } = useTranslation('abac');
  return useMemo<ColumnDef<Policy, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (p) => p.name,
        header: t('table.name'),
        meta: { wrap: true } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <IconShieldLock className="size-4 shrink-0 text-muted-foreground" />
            <div className="min-w-0">
              <div className="truncate font-medium">{row.original.name}</div>
              {row.original.description && (
                <div className="truncate text-xs text-muted-foreground">{row.original.description}</div>
              )}
            </div>
          </div>
        ),
      },
      {
        id: 'effect',
        accessorFn: (p) => t(`effect.${p.effect}`),
        header: t('table.effect'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <Badge variant={row.original.effect === 'deny' ? 'destructive' : 'secondary'}>
            {t(`effect.${row.original.effect}`)}
          </Badge>
        ),
      },
      {
        id: 'resourceType',
        accessorFn: (p) => p.resourceType,
        header: t('table.resourceType'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => <Badge variant="outline">{row.original.resourceType}</Badge>,
      },
      {
        id: 'priority',
        accessorFn: (p) => p.priority,
        header: t('table.priority'),
        meta: { width: 'xs' } satisfies TableColumnMeta,
      },
      {
        id: 'permissions',
        // Sorted and exported by count: the badges are a preview, and a
        // comma-joined permission list is not something to sort a column on.
        accessorFn: (p) => (p.permissions ?? []).length,
        header: t('table.permissions'),
        meta: { width: 'md' } satisfies TableColumnMeta,
        cell: ({ row }) => {
          // Defensive: the backend hydrates `permissions`, but an older
          // server shipping `null` should render an empty cell, not crash.
          const perms = row.original.permissions ?? [];
          if (perms.length === 0) {
            return <span className="text-xs text-muted-foreground">—</span>;
          }
          return (
            <div className="flex max-w-[200px] flex-wrap gap-1">
              {perms.slice(0, 3).map((perm) => (
                <Badge key={perm} variant="outline" className="font-mono text-[10px]">
                  {perm}
                </Badge>
              ))}
              {perms.length > 3 && <span className="text-xs text-muted-foreground">+{perms.length - 3}</span>}
            </div>
          );
        },
      },
      {
        id: 'status',
        accessorFn: (p) => (p.enabled ? t('status.enabled') : t('status.disabled')),
        header: t('table.status'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) =>
          row.original.enabled ? (
            <Badge variant="success">{t('status.enabled')}</Badge>
          ) : (
            <Badge variant="neutral">{t('status.disabled')}</Badge>
          ),
      },
      {
        id: 'actions',
        header: t('table.actions'),
        enableSorting: false,
        meta: { width: 'md', locked: true, noExport: true, label: t('table.actions') } satisfies TableColumnMeta,
        cell: ({ row }) => {
          const p = row.original;
          return (
            <div className="flex justify-end gap-1">
              <Button
                permission={PERM.RBAC_POLICIES_MANAGE}
                variant="ghost"
                size="icon-sm"
                aria-label={p.enabled ? t('actions.disable') : t('actions.enable')}
                onClick={() => toggle.mutate({ id: p.id, enabled: !p.enabled })}
              >
                {p.enabled ? <IconPlayerPause className="size-4" /> : <IconPlayerPlay className="size-4" />}
              </Button>
              <Button
                permission={PERM.RBAC_POLICIES_MANAGE}
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.test')}
                onClick={() => onTest(p)}
              >
                <IconFlask className="size-4" />
              </Button>
              <Button
                permission={PERM.RBAC_POLICIES_MANAGE}
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.edit')}
                onClick={() => onEdit(p)}
              >
                <IconEdit className="size-4" />
              </Button>
              <Button
                permission={PERM.RBAC_POLICIES_MANAGE}
                variant="ghost"
                size="icon-sm"
                aria-label={t('actions.delete')}
                onClick={() => onDelete(p)}
              >
                <IconTrash className="size-4" />
              </Button>
            </div>
          );
        },
      },
    ],
    [t, toggle, onTest, onEdit, onDelete],
  );
}

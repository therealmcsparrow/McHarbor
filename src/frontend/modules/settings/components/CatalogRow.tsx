// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash, IconPencil, IconLock, IconPlugConnected } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Switch } from '@resources/components/ui/Switch';
import { Badge } from '@resources/components/ui/Badge';
import type { Catalog } from '../hooks/useCatalogs';
import { PERM } from '@core/auth/permissionKeys';

type CatalogRowProps = {
  catalog: Catalog;
  togglePending: boolean;
  deletePending: boolean;
  onTest: (catalog: Catalog) => void;
  onEdit: (catalog: Catalog) => void;
  onToggle: (catalog: Catalog) => void;
  onDelete: (catalog: Catalog) => void;
};

/** One catalog source in the per-kind list. */
export function CatalogRow({
  catalog: c,
  togglePending,
  deletePending,
  onTest,
  onEdit,
  onToggle,
  onDelete,
}: CatalogRowProps) {
  const { t } = useTranslation('settings');

  return (
    <li className="flex items-center gap-3 px-4 py-3 text-sm">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="truncate font-medium text-foreground">{c.name}</span>
          {c.readonly ? (
            <Badge variant="secondary" className="gap-1">
              <IconLock className="size-3" />
              {t('catalogs.readonly')}
            </Badge>
          ) : null}
        </div>
        <code className="block truncate text-xs text-muted-foreground">{c.url}</code>
      </div>
      <Button
        permission={PERM.SETTINGS_MANAGE}
        variant="ghost"
        size="icon"
        onClick={() => onTest(c)}
        aria-label={t('catalogs.test.aria', { name: c.name })}
        title={t('catalogs.test.action')}
      >
        <IconPlugConnected className="size-4" />
      </Button>
      {/* Built-ins stay disabled rather than hidden: their name
          and URL are re-canonicalized by SeedDefaults on every
          boot, so an edit would be silently undone at the next
          restart. The title says why instead of leaving a dead
          control unexplained. */}
      <Button
        permission={PERM.SETTINGS_MANAGE}
        variant="ghost"
        size="icon"
        onClick={() => onEdit(c)}
        disabled={c.readonly}
        aria-label={t('catalogs.editAria', { name: c.name })}
        title={c.readonly ? t('catalogs.editReadonlyHint') : t('catalogs.editAction')}
      >
        <IconPencil className="size-4" />
      </Button>
      <Switch checked={c.enabled} onCheckedChange={() => onToggle(c)} disabled={togglePending} aria-label={c.name} />
      <Button
        permission={PERM.SETTINGS_MANAGE}
        variant="ghost"
        size="icon"
        onClick={() => onDelete(c)}
        disabled={c.readonly || deletePending}
        aria-label={t('catalogs.deleteAria', { name: c.name })}
      >
        <IconTrash className="size-4" />
      </Button>
    </li>
  );
}

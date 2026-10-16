// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import type { UserPermissionOverride } from '@resources/hooks/useUsers';

export type CatalogEntry = { key: string; description: string; category: string };

type OverrideRowProps = {
  row: UserPermissionOverride;
  catalog: CatalogEntry[];
  onChange: <K extends keyof UserPermissionOverride>(key: K, value: UserPermissionOverride[K]) => void;
  onRemove: () => void;
};

/** One editable grant/deny override in the user overrides editor. */
export function OverrideRow({ row, catalog, onChange, onRemove }: OverrideRowProps) {
  const { t } = useTranslation('security');

  return (
    <div className="rounded-md border border-border bg-card p-3">
      <div className="grid grid-cols-12 gap-2">
        <div className="col-span-12 md:col-span-5">
          <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.permission')}</label>
          <Select
            value={row.permissionKey}
            onChange={(next) => onChange('permissionKey', next)}
            options={catalog.map((p) => ({
              value: p.key,
              // The key is what gets stored and what `rbac.authorize`
              // matches exactly, so it leads; the description trails it so
              // the search box finds either.
              label: `${p.key} — ${p.description}`,
            }))}
            placeholder={t('users.overrides.permission')}
            ariaLabel={t('users.overrides.permission')}
            variant="outline"
          />
        </div>
        <div className="col-span-6 md:col-span-2">
          <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.effect')}</label>
          <Select
            value={row.effect}
            onChange={(next) => onChange('effect', next as 'grant' | 'deny')}
            options={[
              { value: 'grant', label: t('users.overrides.grant') },
              { value: 'deny', label: t('users.overrides.deny') },
            ]}
            searchable={false}
            ariaLabel={t('users.overrides.effect')}
            variant="outline"
          />
        </div>
        <div className="col-span-6 md:col-span-2">
          <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.expiresAt')}</label>
          <Input
            type="datetime-local"
            value={row.expiresAt?.slice(0, 16) ?? ''}
            onChange={(e) => onChange('expiresAt', e.target.value ? new Date(e.target.value).toISOString() : null)}
          />
        </div>
        <div className="col-span-10 md:col-span-2">
          <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.reason')}</label>
          <Input
            type="text"
            value={row.reason ?? ''}
            onChange={(e) => onChange('reason', e.target.value || null)}
            placeholder={t('users.overrides.reasonPlaceholder')}
          />
        </div>
        <div className="col-span-2 flex items-end justify-end md:col-span-1">
          <Button
            permission={PERM.PERMISSIONS_ASSIGN}
            variant="ghost"
            size="icon"
            onClick={onRemove}
            aria-label={t('users.overrides.remove')}
          >
            <IconTrash className="size-3.5" />
          </Button>
        </div>
      </div>

      <details className="mt-2">
        <summary className="cursor-pointer text-xs text-muted-foreground">{t('users.overrides.advancedScope')}</summary>
        <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-3">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.environmentId')}</label>
            <Input
              type="text"
              value={row.environmentId ?? ''}
              onChange={(e) => onChange('environmentId', e.target.value || null)}
              placeholder={t('users.overrides.environmentPlaceholder')}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.stackName')}</label>
            <Input
              type="text"
              value={row.stackName ?? ''}
              onChange={(e) => onChange('stackName', e.target.value || null)}
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">{t('users.overrides.resourceType')}</label>
            <Input
              type="text"
              value={row.resourceType ?? ''}
              onChange={(e) => onChange('resourceType', e.target.value || null)}
              placeholder={t('users.overrides.resourceTypePlaceholder')}
            />
          </div>
        </div>
      </details>

      {row.effect === 'deny' && (
        <Badge variant="destructive" className="mt-2">
          {t('users.overrides.denyBadge')}
        </Badge>
      )}
    </div>
  );
}

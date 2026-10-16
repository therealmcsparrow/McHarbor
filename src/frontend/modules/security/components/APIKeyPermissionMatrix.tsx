// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Every permission this key asks for, one line per subject. The row model
// (why it is keyed on category, how the four API sets fold) is documented in
// `apiKeyPermissionRows.ts`; the per-row rendering lives in
// `APIKeyPermissionRow.tsx`.

import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconSearch } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { usePermissionCatalog } from '@core/auth/usePermission';
import type { APIKeyDetail } from '../hooks/useAPIKeys';
import { APIKeyPermissionRow } from './APIKeyPermissionRow';
import { buildRows, filterRows, prettify } from './apiKeyPermissionRows';

type APIKeyPermissionMatrixProps = {
  detail: APIKeyDetail;
};

export function APIKeyPermissionMatrix({ detail }: APIKeyPermissionMatrixProps) {
  const { t } = useTranslation('security');
  const { data: catalog = [] } = usePermissionCatalog();
  const [filter, setFilter] = useState('');
  const [blockedOnly, setBlockedOnly] = useState(false);

  const rows = useMemo(
    () =>
      buildRows(detail, catalog, (category) =>
        // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: permission categories come from the backend catalog, including plugin-declared ones
        t(`permissions.${category}`, { defaultValue: prettify(category) }),
      ),
    [detail, catalog, t],
  );

  const total = rows.reduce((sum, row) => sum + row.actions.length, 0);
  const blockedCount = rows.reduce((sum, row) => sum + row.blocked, 0);
  const wildcard = detail.configuredPermissions.includes('*');

  const visible = useMemo(() => filterRows(rows, filter, blockedOnly), [rows, filter, blockedOnly]);

  return (
    <div className="flex flex-col gap-3">
      {wildcard && (
        <Alert
          variant="warning"
          title={t('apiKeys.detail.matrixWildcardTitle')}
          description={t('apiKeys.detail.matrixWildcardBody')}
        />
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[14rem] flex-1">
          <IconSearch
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            value={filter}
            onChange={(evt) => setFilter(evt.target.value)}
            placeholder={t('apiKeys.detail.matrixSearch')}
            aria-label={t('apiKeys.detail.matrixSearch')}
            className="pl-8"
          />
        </div>
        <span className="text-xs text-muted-foreground tabular-nums">
          {t('apiKeys.detail.matrixCount', { count: total })}
        </span>
        {blockedCount > 0 && (
          <Button
            variant={blockedOnly ? 'secondary' : 'ghost'}
            size="sm"
            aria-pressed={blockedOnly}
            onClick={() => setBlockedOnly((previous) => !previous)}
          >
            {t('apiKeys.detail.matrixBlocked', { count: blockedCount })}
          </Button>
        )}
      </div>

      {total === 0 ? (
        <p className="rounded-lg border border-border px-3 py-6 text-center text-sm text-muted-foreground">
          {wildcard ? t('apiKeys.detail.matrixWildcardOnly') : t('apiKeys.detail.matrixNone')}
        </p>
      ) : visible.length === 0 ? (
        <p className="rounded-lg border border-border px-3 py-6 text-center text-sm text-muted-foreground">
          {t('apiKeys.detail.matrixEmpty')}
        </p>
      ) : (
        <ul className="overflow-hidden rounded-lg border border-border">
          {visible.map((row) => (
            <APIKeyPermissionRow key={row.category} row={row} />
          ))}
        </ul>
      )}
    </div>
  );
}

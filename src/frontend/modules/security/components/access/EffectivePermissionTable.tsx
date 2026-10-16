// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import type { EffectivePermissions } from '../../hooks/useAccessReview';
import { PermissionSourceList } from './PermissionSourceList';

type Props = {
  data: EffectivePermissions;
  filter: string;
  showAll: boolean;
};

export function EffectivePermissionTable({ data, filter, showAll }: Props) {
  const { t } = useTranslation('access');
  const needle = filter.trim().toLowerCase();
  const rows = data.permissions.filter(
    (p) =>
      (showAll || p.effective || p.authorizeGrant || p.sources.length > 0) &&
      (needle === '' || p.key.toLowerCase().includes(needle)),
  );
  if (rows.length === 0) {
    return <p className="py-6 text-center text-sm text-muted-foreground">{t('effective.noMatches')}</p>;
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 font-medium">{t('effective.columns.permission')}</th>
            <th className="px-3 py-2 font-medium">{t('effective.columns.held')}</th>
            <th className="px-3 py-2 font-medium">{t('effective.columns.authorize')}</th>
            <th className="px-3 py-2 font-medium">{t('effective.columns.sources')}</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {rows.map((p) => (
            <tr key={p.key}>
              <td className="px-3 py-2 font-mono text-xs text-foreground">{p.key}</td>
              <td className="px-3 py-2">
                <Badge variant={p.effective ? 'success' : 'neutral'}>
                  {p.effective ? t('effective.held') : t('effective.notHeld')}
                </Badge>
              </td>
              <td className="px-3 py-2">
                <Badge variant={p.authorizeGrant ? 'success' : 'neutral'}>
                  {p.authorizeGrant ? t('effective.held') : t('effective.notHeld')}
                </Badge>
              </td>
              <td className="px-3 py-2">
                <PermissionSourceList sources={p.sources} environmentNames={data.environmentNames} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Link } from 'react-router';
import { Badge } from '@resources/components/ui/Badge';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatDate } from '@resources/utils/format';
import type { AuditDenial } from '../hooks/useAuditCenter';

type AuditDenialsTableProps = {
  rows: AuditDenial[];
  envName: (id: string | null, storedName?: string | null) => string;
};

function DenialActor({ row }: { row: AuditDenial }) {
  const { t } = useTranslation('audit');
  const itemName = row.username || row.userId || t('denials.unknownActor');
  if (!row.userId) return <span className="text-muted-foreground">{itemName}</span>;
  return (
    <Link
      to={`/security/users/${row.userId}`}
      className="text-primary underline-offset-2 hover:underline"
      aria-label={t('denials.openUser', { name: itemName })}
    >
      {itemName}
    </Link>
  );
}

export function AuditDenialsTable({ rows, envName }: AuditDenialsTableProps) {
  const { t } = useTranslation('audit');

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('denials.columnTime')}</TableHead>
          <TableHead>{t('denials.columnActor')}</TableHead>
          <TableHead>{t('denials.columnPermission')}</TableHead>
          <TableHead>{t('denials.columnRoute')}</TableHead>
          <TableHead>{t('denials.columnEnvironment')}</TableHead>
          <TableHead>{t('denials.columnSource')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.id}>
            <TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
              {formatDate(row.timestamp)}
            </TableCell>
            <TableCell>
              <DenialActor row={row} />
            </TableCell>
            <TableCell className="font-mono text-xs">{row.permission}</TableCell>
            <TableCell className="font-mono text-xs break-all text-muted-foreground">
              {row.route || (row.policyId ? t('denials.policy', { id: row.policyId }) : '')}
            </TableCell>
            <TableCell className="text-xs">
              {row.environmentId ? envName(row.environmentId, row.environmentName) : t('denials.global')}
            </TableCell>
            <TableCell>
              <Badge variant={row.kind === 'abac' ? 'info' : 'warning'}>{t(`denials.kind.${row.kind}`)}</Badge>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { formatDate } from '@resources/utils/format';
import type { AuditEntry } from '@resources/components/AuditEntryDetails';

export function AIToolEventsTable({ rows }: { rows: AuditEntry[] }) {
  const { t } = useTranslation('audit');

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('denials.columnTime')}</TableHead>
          <TableHead>{t('denials.columnActor')}</TableHead>
          <TableHead>{t('ai.columnEvent')}</TableHead>
          <TableHead>{t('ai.columnTool')}</TableHead>
          <TableHead>{t('ai.columnDetails')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.id}>
            <TableCell className="font-mono text-xs whitespace-nowrap text-muted-foreground">
              {formatDate(row.timestamp)}
            </TableCell>
            <TableCell className="text-xs">{row.username ?? ''}</TableCell>
            <TableCell className="font-mono text-xs">{row.action}</TableCell>
            <TableCell className="font-mono text-xs">{row.entityName ?? ''}</TableCell>
            <TableCell className="font-mono text-xs break-all text-muted-foreground">{row.details ?? ''}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

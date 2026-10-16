// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBox, IconListCheck } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { EmptyState } from '@resources/components/ui/EmptyState';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { list } from '../format';
import type { Row } from '../types';
import { badgeVariant, stateTone } from './kubernetes-facets';

export function ContainersPanel({ rows }: { rows: Row[] }) {
  const { t } = useTranslation('kubernetes');

  if (rows.length === 0) {
    return <EmptyState variant="info" icon={<IconBox className="size-6" />} title={t('detail.empty.containers')} />;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('sections.containers')}</CardTitle>
      </CardHeader>
      <CardContent className="px-0 pb-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('columns.name')}</TableHead>
              <TableHead>{t('columns.image')}</TableHead>
              <TableHead>{t('columns.state')}</TableHead>
              <TableHead>{t('columns.ready')}</TableHead>
              <TableHead className="text-right">{t('columns.restarts')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={list(row.name)}>
                <TableCell className="font-medium">{list(row.name)}</TableCell>
                {/* The image reference is the one field here that routinely
                    outruns its column, and it is an identifier rather than
                    prose — mono, and allowed to wrap rather than be cut. */}
                <TableCell className="font-mono text-[11px] break-all text-muted-foreground">
                  {list(row.image)}
                </TableCell>
                <TableCell>
                  <Badge variant={badgeVariant(String(row.state))}>{list(row.state)}</Badge>
                </TableCell>
                <TableCell>
                  <StatusIndicator
                    tone={row.ready === true ? 'success' : 'warning'}
                    label={row.ready === true ? t('values.yes') : t('values.no')}
                    size="sm"
                  />
                </TableCell>
                <TableCell className="text-right tabular-nums">{list(row.restartCount)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

export function ConditionsPanel({ rows }: { rows: Row[] }) {
  const { t } = useTranslation('kubernetes');

  if (rows.length === 0) {
    return (
      <EmptyState variant="info" icon={<IconListCheck className="size-6" />} title={t('detail.empty.conditions')} />
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">{t('sections.conditions')}</CardTitle>
      </CardHeader>
      <CardContent className="px-0 pb-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('columns.type')}</TableHead>
              <TableHead>{t('columns.status')}</TableHead>
              <TableHead>{t('columns.message')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={`${list(row.type)}-${list(row.status)}`}>
                <TableCell className="font-medium">{list(row.type)}</TableCell>
                <TableCell>
                  <StatusIndicator tone={stateTone(String(row.status))} label={list(row.status)} size="sm" />
                </TableCell>
                {/* A condition message is the only free prose the API returns
                    and is regularly a full sentence, so it wraps instead of
                    pushing the table sideways. */}
                <TableCell className="break-words whitespace-normal text-muted-foreground">
                  {list(row.message)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

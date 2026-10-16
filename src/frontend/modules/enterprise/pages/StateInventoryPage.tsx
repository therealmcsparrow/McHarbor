// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent } from '@resources/components/ui/Card';
import { Grid } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@resources/components/ui/Table';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { Hint } from '@resources/components/ui/Hint';
import { PageHeader } from '@resources/layout/PageHeader';
import { formatBytes } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { type Inventory, type InventoryEntry, useStateInventory } from '@resources/hooks/useSupport';

const backupTone: Record<InventoryEntry['backup'], 'success' | 'warning' | 'secondary' | 'info'> = {
  included: 'success',
  partial: 'warning',
  excluded: 'secondary',
  database: 'info',
};

const cleanupTone: Record<InventoryEntry['cleanup'], 'success' | 'secondary' | 'info' | 'warning'> = {
  no: 'secondary',
  safe: 'success',
  managed: 'info',
  regenerable: 'info',
  unknown: 'warning',
};

export default function StateInventoryPage() {
  const { t } = useTranslation('support');
  const canView = usePermission(PERM.SUPPORT_VIEW);
  const inventory = useStateInventory(canView);

  return (
    <div className="space-y-6">
      <PageHeader title={t('inventory.title')} description={t('inventory.description')} headingLeaf />
      {!canView ? (
        <Alert variant="warning">{t('permission.requires', { ns: 'common', permission: PERM.SUPPORT_VIEW })}</Alert>
      ) : inventory.isLoading ? (
        <Spinner />
      ) : inventory.error ? (
        <Alert variant="destructive">{inventory.error.message}</Alert>
      ) : inventory.data ? (
        <InventoryView inventory={inventory.data} />
      ) : null}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="space-y-1">
        <TypographyText tone="muted">{label}</TypographyText>
        <TypographyText weight="semibold" className="break-all">
          {value}
        </TypographyText>
      </CardContent>
    </Card>
  );
}

function InventoryView({ inventory }: { inventory: Inventory }) {
  const { t } = useTranslation('support');
  return (
    <>
      <Grid columns="grid-cols-1 md:grid-cols-2 xl:grid-cols-4" gap={4}>
        <Fact label={t('inventory.dataDir')} value={inventory.dataDir} />
        <Fact label={t('inventory.used')} value={formatBytes(inventory.totalBytes)} />
        <Fact
          label={t('inventory.free')}
          value={
            inventory.diskBytes ? `${formatBytes(inventory.freeBytes)} / ${formatBytes(inventory.diskBytes)}` : '-'
          }
        />
        <Fact label={t('inventory.driver')} value={inventory.databaseDriver} />
      </Grid>
      <Card>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('inventory.columns.category')}</TableHead>
                <TableHead>{t('inventory.columns.path')}</TableHead>
                <TableHead className="text-right">{t('inventory.columns.size')}</TableHead>
                <TableHead>{t('inventory.columns.writable')}</TableHead>
                <TableHead>{t('inventory.columns.backup')}</TableHead>
                <TableHead>{t('inventory.columns.cleanup')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {inventory.entries.map((e) => (
                <InventoryRow key={`${e.category}:${e.path}`} entry={e} />
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </>
  );
}

function InventoryRow({ entry }: { entry: InventoryEntry }) {
  const { t } = useTranslation('support');
  return (
    <TableRow>
      <TableCell className="font-medium text-foreground">{t(`inventory.category.${entry.category}`)}</TableCell>
      <TableCell className="font-mono">{entry.path}</TableCell>
      <TableCell className="text-right tabular-nums">
        {entry.exists ? (
          <Hint text={entry.truncated ? t('inventory.truncated') : undefined}>
            <span>
              {formatBytes(entry.sizeBytes)}
              {entry.truncated ? '+' : ''}
              {entry.kind === 'dir' && (
                <span className="ml-2 text-muted-foreground">{t('inventory.files', { count: entry.files })}</span>
              )}
            </span>
          </Hint>
        ) : (
          <span className="text-muted-foreground">{t('inventory.missing')}</span>
        )}
      </TableCell>
      <TableCell>
        {!entry.exists ? (
          <span className="text-muted-foreground">-</span>
        ) : entry.writable ? (
          <IconCheck className="size-4 text-success" aria-label={t('inventory.yes')} />
        ) : (
          <IconX className="size-4 text-destructive" aria-label={t('inventory.no')} />
        )}
      </TableCell>
      <TableCell>
        <Badge variant={backupTone[entry.backup]}>{t(`inventory.backup.${entry.backup}`)}</Badge>
      </TableCell>
      <TableCell>
        <Badge variant={cleanupTone[entry.cleanup]}>{t(`inventory.cleanup.${entry.cleanup}`)}</Badge>
      </TableCell>
    </TableRow>
  );
}

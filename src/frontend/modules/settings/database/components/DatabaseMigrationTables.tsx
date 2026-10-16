// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';
import type { MigrationStatus } from '../types';

type DatabaseMigrationTablesProps = {
  status: MigrationStatus | null | undefined;
};

/** The per-table row counters and the warning list, in copy order. */
export function DatabaseMigrationTables({ status: currentStatus }: DatabaseMigrationTablesProps) {
  const { t } = useTranslation('database');
  if (!currentStatus) return null;

  return (
    <>
      {currentStatus.tables.length > 0 && (
        <Stack gap={1}>
          <TypographyText weight="medium">{t('migration.tables')}</TypographyText>
          <Box className="rounded-lg border border-border">
            {currentStatus.tables.map((table) => (
              <Flex
                key={table.table}
                align="center"
                justify="between"
                gap={2}
                className={cn('border-b border-border px-3 py-1.5 last:border-b-0', !table.done && 'bg-muted/30')}
              >
                <TypographyText size="sm" className="truncate font-mono">
                  {table.table}
                </TypographyText>
                <TypographyText size="xs" tone="muted">
                  {new Intl.NumberFormat().format(table.rows)} / {new Intl.NumberFormat().format(table.total)}
                </TypographyText>
              </Flex>
            ))}
          </Box>
        </Stack>
      )}

      {currentStatus.warnings && currentStatus.warnings.length > 0 && (
        <Stack gap={1} className="rounded-lg border border-warning/40 bg-warning/5 p-3">
          <Flex align="center" gap={2}>
            <IconAlertTriangle className="size-4 text-warning" aria-hidden />
            <TypographyText weight="medium">{t('migration.warnings')}</TypographyText>
          </Flex>
          {currentStatus.warnings.map((warning) => (
            <TypographyText key={warning} size="sm" tone="muted">
              {warning}
            </TypographyText>
          ))}
        </Stack>
      )}
    </>
  );
}

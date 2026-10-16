// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconBrandMysql,
  IconCheck,
  IconFileDatabase,
  IconServer2,
  IconServerBolt,
  type TablerIcon,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { RadioCard } from '../../components/RadioCard';
import { cn } from '@resources/utils/cn';
import type { DatabaseDriver } from '../types';

const DRIVER_ICONS: Record<DatabaseDriver, TablerIcon> = {
  sqlite: IconFileDatabase,
  postgres: IconServerBolt,
  mysql: IconBrandMysql,
  mssql: IconServer2,
};

type DatabaseDriverPickerProps = {
  value: DatabaseDriver;
  onChange: (driver: DatabaseDriver) => void;
  /** Which drivers to offer. Comes from the backend so the two stay in step. */
  drivers: DatabaseDriver[];
  /** The driver McHarbor is running on, marked as current in the list. */
  current?: DatabaseDriver;
  disabled?: boolean;
  /** Permission that gates changing the choice. */
  permission?: string;
};

/**
 * The backends as cards rather than a dropdown.
 *
 * Choosing a database is a decision with consequences an operator should be
 * able to read before making it — SQLite needs no server and cannot run
 * multi-node; Postgres and MySQL need a server and unlock it. A select
 * element can hold a few words; it cannot hold that.
 */
export function DatabaseDriverPicker({
  value,
  onChange,
  drivers,
  current,
  disabled,
  permission,
}: DatabaseDriverPickerProps) {
  const { t } = useTranslation('database');
  const groupName = useId();

  return (
    <Flex gap={3} wrap role="radiogroup" aria-label={t('picker.label')}>
      {drivers.map((driver) => {
        const Icon = DRIVER_ICONS[driver];
        const selected = driver === value;
        return (
          <RadioCard
            key={driver}
            name={groupName}
            permission={permission}
            checked={selected}
            disabled={disabled}
            onSelect={() => onChange(driver)}
            className="min-w-[200px] flex-1 items-start justify-start p-4 text-left"
          >
            <Flex align="start" gap={3} className="w-full">
              <Box
                className={cn(
                  'rounded-lg p-2',
                  selected ? 'bg-primary/10 text-primary' : 'bg-muted text-muted-foreground',
                )}
              >
                <Icon className="size-5" />
              </Box>
              <Stack gap={1} className="min-w-0 flex-1">
                <Flex align="center" gap={2} wrap>
                  <TypographyText weight="semibold">{t(`driver.${driver}.name`)}</TypographyText>
                  {driver === current && <Badge variant="secondary">{t('picker.current')}</Badge>}
                  {selected && <IconCheck className="size-4 text-primary" aria-hidden />}
                </Flex>
                <TypographyText size="sm" tone="muted">
                  {t(`driver.${driver}.description`)}
                </TypographyText>
              </Stack>
            </Flex>
          </RadioCard>
        );
      })}
    </Flex>
  );
}

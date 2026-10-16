// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { Box, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { DEFAULT_PORTS, SSL_OPTIONS, type DatabaseConnectionInput, type DatabaseDriver } from '../types';
import { DatabaseConnectionFields } from './DatabaseConnectionFields';

const DSN_PLACEHOLDERS: Record<DatabaseDriver, string> = {
  sqlite: '',
  postgres: 'postgres://user:password@host:5432/mcharbor?sslmode=require',
  mysql: 'user:password@tcp(host:3306)/mcharbor?parseTime=true&loc=UTC',
  mssql: 'sqlserver://user:password@host:1433?database=mcharbor&encrypt=true',
};

type DatabaseConnectionFormProps = {
  value: DatabaseConnectionInput;
  onChange: (next: DatabaseConnectionInput) => void;
  /** True while the raw-DSN escape hatch is showing. */
  rawMode: boolean;
  onRawModeChange: (raw: boolean) => void;
  disabled?: boolean;
};

/**
 * The connection form.
 *
 * Two modes. The structured one is the default because it is the one that
 * can be checked field by field and prefilled with a sensible port. The raw
 * DSN is there because managed Postgres and MySQL services routinely hand
 * out connection strings with parameters no form models — an endpoint id, a
 * client certificate, a non-default search_path — and a form that cannot
 * accept those is a form the operator works around by editing Compose,
 * which is the thing this screen exists to replace.
 */
const SQLITE_PATH_PLACEHOLDER = './data/mcharbor.db';

export function DatabaseConnectionForm({
  value,
  onChange,
  rawMode,
  onRawModeChange,
  disabled,
}: DatabaseConnectionFormProps) {
  const { t } = useTranslation('database');
  const driver = value.driver;

  const set = (patch: Partial<DatabaseConnectionInput>) => onChange({ ...value, ...patch });

  if (driver === 'sqlite') {
    return (
      <Stack gap={2}>
        <Label htmlFor="db-path">{t('form.path')}</Label>
        <Input
          variant="outline"
          id="db-path"
          value={value.path ?? ''}
          disabled={disabled}
          placeholder={SQLITE_PATH_PLACEHOLDER}
          onChange={(e) => set({ path: e.target.value })}
        />
        <TypographyText size="sm" tone="muted">
          {t('form.pathHelp')}
        </TypographyText>
      </Stack>
    );
  }

  return (
    <Stack gap={4}>
      <Box className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
        <Stack gap={1}>
          <TypographyText weight="medium">{t('form.rawMode')}</TypographyText>
          <TypographyText size="sm" tone="muted">
            {t('form.rawModeHelp')}
          </TypographyText>
        </Stack>
        <Switch
          checked={rawMode}
          disabled={disabled}
          onCheckedChange={onRawModeChange}
          aria-label={t('form.rawMode')}
        />
      </Box>

      {rawMode ? (
        <Stack gap={2}>
          <Label htmlFor="db-dsn">{t('form.dsn')}</Label>
          <Input
            variant="outline"
            id="db-dsn"
            value={value.dsn ?? ''}
            disabled={disabled}
            placeholder={DSN_PLACEHOLDERS[driver]}
            onChange={(e) => set({ dsn: e.target.value })}
          />
          <TypographyText size="sm" tone="muted">
            {t('form.dsnHelp')}
          </TypographyText>
        </Stack>
      ) : (
        <DatabaseConnectionFields value={value} onPatch={set} disabled={disabled} />
      )}

      <TypographyText size="sm" tone="muted">
        {t('form.credentialsStored')}
      </TypographyText>
    </Stack>
  );
}

/** A blank form for a driver, with that driver's default port filled in. */
export function emptyConnection(driver: DatabaseDriver): DatabaseConnectionInput {
  return {
    driver,
    host: '',
    port: DEFAULT_PORTS[driver] || undefined,
    name: '',
    user: '',
    password: '',
    sslMode: SSL_OPTIONS[driver][0],
    trustServerCertificate: false,
    dsn: '',
    path: '',
  };
}

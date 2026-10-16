// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Box, Flex, Grid, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { DEFAULT_PORTS, SSL_OPTIONS, type DatabaseConnectionInput, type DatabaseDriver } from '../types';

const SSL_LABELS: Record<DatabaseDriver, string> = {
  sqlite: 'form.tls',
  postgres: 'form.sslMode',
  mysql: 'form.tls',
  mssql: 'form.encrypt',
};

type DatabaseConnectionFieldsProps = {
  value: DatabaseConnectionInput;
  onPatch: (patch: Partial<DatabaseConnectionInput>) => void;
  disabled?: boolean;
};

/**
 * The structured half of the connection form: host, port, database, user,
 * password and the driver's transport-security setting.
 *
 * Split out of DatabaseConnectionForm so that component stays about the
 * choice between structured entry and a raw DSN, and this one stays about
 * the fields themselves.
 */
export function DatabaseConnectionFields({ value, onPatch, disabled }: DatabaseConnectionFieldsProps) {
  const { t } = useTranslation('database');
  const driver = value.driver;
  const sslLabel = t(SSL_LABELS[driver]);

  return (
    <Stack gap={4}>
      <Grid columns="grid-cols-1 sm:grid-cols-3" gap={3}>
        <Box className="sm:col-span-2">
          <Label htmlFor="db-host" className="mb-1">
            {t('form.host')}
          </Label>
          <Input
            variant="outline"
            id="db-host"
            value={value.host ?? ''}
            disabled={disabled}
            placeholder={driver}
            onChange={(e) => onPatch({ host: e.target.value })}
          />
        </Box>
        <Box>
          <Label htmlFor="db-port" className="mb-1">
            {t('form.port')}
          </Label>
          <NumberInput
            id="db-port"
            min={1}
            max={65535}
            disabled={disabled}
            value={value.port ?? DEFAULT_PORTS[driver]}
            onChange={(next) => onPatch({ port: next })}
          />
        </Box>
      </Grid>

      <Box>
        <Label htmlFor="db-name" className="mb-1">
          {t('form.database')}
        </Label>
        <Input
          variant="outline"
          id="db-name"
          value={value.name ?? ''}
          disabled={disabled}
          placeholder={t('form.databasePlaceholder')}
          onChange={(e) => onPatch({ name: e.target.value })}
        />
      </Box>

      <Grid columns="grid-cols-1 sm:grid-cols-2" gap={3}>
        <Box>
          <Label htmlFor="db-user" className="mb-1">
            {t('form.user')}
          </Label>
          <Input
            variant="outline"
            id="db-user"
            value={value.user ?? ''}
            disabled={disabled}
            autoComplete="off"
            onChange={(e) => onPatch({ user: e.target.value })}
          />
        </Box>
        <Box>
          <Label htmlFor="db-password" className="mb-1">
            {t('form.password')}
          </Label>
          <PasswordInput
            id="db-password"
            value={value.password ?? ''}
            disabled={disabled}
            autoComplete="new-password"
            onChange={(e) => onPatch({ password: e.target.value })}
          />
        </Box>
      </Grid>

      <Box>
        <Label htmlFor="db-ssl" className="mb-1">
          {sslLabel}
        </Label>
        <Select
          value={value.sslMode ?? SSL_OPTIONS[driver][0] ?? ''}
          disabled={disabled}
          ariaLabel={sslLabel}
          onChange={(next) => onPatch({ sslMode: next })}
          options={SSL_OPTIONS[driver].map((option) => ({
            value: option,
            label: option,
          }))}
        />
        <TypographyText size="sm" tone="muted" className="mt-1">
          {t('form.sslHelp')}
        </TypographyText>
      </Box>

      {driver === 'mssql' && (
        <Flex align="center" justify="between" gap={3} className="rounded-lg border border-border bg-muted/20 p-3">
          <Stack gap={1}>
            <TypographyText weight="medium">{t('form.trustServerCertificate')}</TypographyText>
            <TypographyText size="sm" tone="muted">
              {t('form.trustServerCertificateHelp')}
            </TypographyText>
          </Stack>
          <Switch
            checked={value.trustServerCertificate ?? false}
            disabled={disabled}
            onCheckedChange={(next) => onPatch({ trustServerCertificate: next })}
            aria-label={t('form.trustServerCertificate')}
          />
        </Flex>
      )}

      {driver === 'mssql' && (
        <TypographyText size="sm" tone="muted">
          {t('form.mssqlSnapshot')}
        </TypographyText>
      )}
    </Stack>
  );
}

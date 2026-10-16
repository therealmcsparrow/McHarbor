// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconArrowsExchange, IconDatabaseImport, IconPlugConnected } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Switch } from '@resources/components/ui/Switch';
import { TableSkeleton } from '@resources/components/ui/Skeleton';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { useDatabaseMigrationPanel } from '../hooks/useDatabaseMigrationPanel';
import { DatabaseConnectionForm } from './DatabaseConnectionForm';
import { DatabaseCleanupCard } from './DatabaseCleanupCard';
import { DatabaseCurrentCard } from './DatabaseCurrentCard';
import { DatabaseDriverPicker } from './DatabaseDriverPicker';
import { DatabaseHealthCard } from './DatabaseHealthCard';
import { DatabaseMigrationDialog } from './DatabaseMigrationDialog';
import { DatabaseTestSteps } from './DatabaseTestSteps';

/**
 * Settings → System → Database.
 *
 * Three things in one place: what McHarbor is running on, a form for
 * somewhere else, and the button that moves every row there. The test has to
 * pass before the migrate button becomes available — not because the backend
 * would skip its own pre-flight (it re-runs the same seven steps at the
 * start of the job), but because an operator should never be one click from
 * a data migration they have not verified the destination for.
 */
export function DatabaseSettingsPanel() {
  const { t } = useTranslation('database');
  const {
    config,
    test,
    startMigration,
    cancelMigration,
    migration,
    driver,
    isCurrent,
    blocked,
    moveSameKind,
    setMoveSameKind,
    setDriver,
    setConnection,
    rawMode,
    setRawMode,
    restartAfter,
    setRestartAfter,
    dialogOpen,
    closeDialog,
    target,
    readOnly,
    jobActive,
    canMigrate,
    runTest,
    runMigration,
  } = useDatabaseMigrationPanel();

  if (config.isLoading) return <TableSkeleton rows={4} columns={3} />;
  if (config.isError || !config.data) {
    return <ErrorState title={t('errorTitle')} error={config.error} onRetry={() => config.refetch()} />;
  }

  return (
    <Stack gap={5}>
      <DatabaseCurrentCard config={config.data} />

      <DatabaseHealthCard />

      <DatabaseCleanupCard />

      <Card>
        <CardHeader>
          <CardTitle>{t('choose.title')}</CardTitle>
          <TypographyText size="sm" tone="muted">
            {t('choose.description')}
          </TypographyText>
        </CardHeader>
        <CardContent>
          <Stack gap={5}>
            <DatabaseDriverPicker
              value={driver}
              onChange={setDriver}
              drivers={config.data.availableDrivers}
              current={config.data.driver}
              disabled={readOnly || jobActive}
              permission={PERM.DATABASE_MANAGE}
            />

            {blocked ? (
              <Flex align="start" gap={3} className="rounded-lg border border-dashed border-border bg-muted/20 p-3">
                <IconArrowsExchange className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
                <Stack gap={2}>
                  <TypographyText size="sm" tone="muted">
                    {t('choose.inUse', { driver: t(`driver.${driver}.name`) })}
                  </TypographyText>
                  <div>
                    <Button
                      variant="outline"
                      size="sm"
                      permission={PERM.DATABASE_MANAGE}
                      disabled={readOnly || jobActive}
                      onClick={() => setMoveSameKind(true)}
                    >
                      {t('choose.moveSameKind', { driver: t(`driver.${driver}.name`) })}
                    </Button>
                  </div>
                </Stack>
              </Flex>
            ) : (
              <>
                {isCurrent && moveSameKind && (
                  <TypographyText size="sm" tone="muted">
                    {t('choose.sameKindHint')}
                  </TypographyText>
                )}
                <DatabaseConnectionForm
                  value={target}
                  onChange={setConnection}
                  rawMode={rawMode}
                  onRawModeChange={setRawMode}
                  disabled={readOnly || jobActive}
                />
              </>
            )}

            <Flex align="center" gap={2} wrap>
              <Button
                permission={PERM.DATABASE_MANAGE}
                variant="outline"
                disabled={readOnly || test.isPending || jobActive || blocked}
                onClick={runTest}
              >
                <IconPlugConnected className="size-4" aria-hidden />
                {test.isPending ? t('test.running') : t('test.run')}
              </Button>

              <Button
                permission={PERM.DATABASE_MANAGE}
                disabled={!canMigrate || startMigration.isPending}
                onClick={runMigration}
              >
                <IconDatabaseImport className="size-4" aria-hidden />
                {t('migration.start')}
              </Button>
            </Flex>

            {(test.data || test.isPending) && (
              <Stack gap={2}>
                <TypographyText weight="medium">{t('test.title')}</TypographyText>
                <DatabaseTestSteps result={test.data} running={test.isPending} />
              </Stack>
            )}

            <Box className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
              <Stack gap={1}>
                <TypographyText weight="medium">{t('choose.restartAfter')}</TypographyText>
                <TypographyText size="sm" tone="muted">
                  {config.data.restartSupported ? t('choose.restartAfterHelp') : t('current.noRestart')}
                </TypographyText>
              </Stack>
              <Switch
                checked={restartAfter && config.data.restartSupported}
                disabled={!config.data.restartSupported || readOnly || jobActive}
                onCheckedChange={setRestartAfter}
                aria-label={t('choose.restartAfter')}
              />
            </Box>

            <Alert variant="warning" title={t('choose.warningTitle')}>
              <Stack gap={2}>
                <span>{t('choose.warningBody')}</span>
                <span>{t('choose.warningOldDatabase')}</span>
              </Stack>
            </Alert>
          </Stack>
        </CardContent>
      </Card>

      <DatabaseMigrationDialog
        open={dialogOpen}
        status={migration.data}
        cancelling={cancelMigration.isPending}
        onCancel={() => cancelMigration.mutate()}
        onClose={closeDialog}
      />
    </Stack>
  );
}

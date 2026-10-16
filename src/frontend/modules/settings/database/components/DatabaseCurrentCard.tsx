// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconLock } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { Box, Flex, Grid, Stack } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatBytes } from '@resources/utils/format';
import type { DatabaseConfig } from '../types';

type DatabaseCurrentCardProps = {
  config: DatabaseConfig;
};

/**
 * What McHarbor is running on right now, and where that decision came from.
 *
 * The `source` row is the one that matters most: an operator looking at a
 * form that does nothing needs to be told that an environment variable is
 * overriding it, on the same screen, rather than discovering it after a
 * restart puts the old database back.
 */
export function DatabaseCurrentCard({ config }: DatabaseCurrentCardProps) {
  const { t } = useTranslation('database');

  return (
    <Card>
      <CardHeader>
        <Flex align="center" justify="between" gap={3} wrap>
          <CardTitle>{t('current.title')}</CardTitle>
          <Flex align="center" gap={2}>
            <Badge variant="default">{t(`driver.${config.driver}.name`)}</Badge>
            <Badge variant="secondary">{t(`current.source.${config.source}`)}</Badge>
          </Flex>
        </Flex>
      </CardHeader>
      <CardContent>
        <Stack gap={4}>
          {config.envPinned && (
            <Flex align="start" gap={3} className="rounded-lg border border-warning/40 bg-warning/5 p-3">
              <IconLock className="mt-0.5 size-5 shrink-0 text-warning" aria-hidden />
              <Stack gap={1}>
                <TypographyText weight="medium">{t('current.envPinnedTitle')}</TypographyText>
                <TypographyText size="sm" tone="muted">
                  {t('current.envPinnedBody')}
                </TypographyText>
              </Stack>
            </Flex>
          )}

          <Grid columns="grid-cols-1 sm:grid-cols-2 lg:grid-cols-4" gap={3}>
            <Stat label={t('current.target')} value={config.target} mono />
            <Stat label={t('current.version')} value={config.serverVersion ?? '—'} />
            <Stat label={t('current.tables')} value={new Intl.NumberFormat().format(config.tables)} />
            <Stat
              label={t('current.rows')}
              value={
                config.sizeBytes
                  ? `${new Intl.NumberFormat().format(config.rows)} · ${formatBytes(config.sizeBytes)}`
                  : new Intl.NumberFormat().format(config.rows)
              }
            />
          </Grid>

          {!config.restartSupported && (
            <Flex align="start" gap={3} className="rounded-lg border border-border bg-muted/20 p-3">
              <IconAlertTriangle className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
              <TypographyText size="sm" tone="muted">
                {t('current.noRestart')}
              </TypographyText>
            </Flex>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

/** One labelled figure in the summary grid. */
function Stat({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <Box className="rounded-lg border border-border bg-muted/20 p-3">
      <TypographyText size="xs" tone="muted">
        {label}
      </TypographyText>
      <TypographyText className={mono ? 'mt-1 font-mono break-all' : 'mt-1'} weight="medium">
        {value}
      </TypographyText>
    </Box>
  );
}

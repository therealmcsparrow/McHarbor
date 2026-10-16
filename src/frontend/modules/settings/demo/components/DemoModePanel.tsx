// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconInfoCircle } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { TableSkeleton } from '@resources/components/ui/Skeleton';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { demoSizesOf } from '@resources/constants/demoConfig';
import { useDemoStatus } from '@resources/hooks/useDemoMode';
import type { DemoStatus } from '@resources/types/demo';
import { DemoActionControl } from './DemoActionControl';
import { DemoEstateList } from './DemoEstateList';
import { DemoSizesControl } from './DemoSizesControl';

function DemoStateRows({ status: currentStatus }: { status: DemoStatus }) {
  const { t } = useTranslation('demo');
  return (
    <div>
      <InfoRow label={t('state.label')}>
        {currentStatus.active
          ? t('state.activeSince', { date: formatDate(currentStatus.enteredAt) })
          : t('state.inactive')}
      </InfoRow>
      <InfoRow label={t('state.data')}>
        {currentStatus.databaseExists ? t('state.dataPresent') : t('state.dataAbsent')}
      </InfoRow>
    </div>
  );
}

function DemoNotes() {
  const { t } = useTranslation('demo');
  return (
    <Flex align="start" gap={3} className="rounded-lg border border-border bg-muted/20 p-3">
      <IconInfoCircle className="mt-0.5 size-5 shrink-0 text-muted-foreground" aria-hidden />
      <Stack gap={1}>
        <TypographyText size="sm">{t('notes.realData')}</TypographyText>
        <TypographyText size="sm" tone="muted">
          {t('notes.restart')}
        </TypographyText>
        <TypographyText size="sm" tone="muted">
          {t('notes.disabled')}
        </TypographyText>
      </Stack>
    </Flex>
  );
}

export function DemoModePanel() {
  const { t } = useTranslation('demo');
  const currentStatus = useDemoStatus();

  if (currentStatus.isLoading) return <TableSkeleton rows={3} columns={2} />;
  if (currentStatus.isError || !currentStatus.data) {
    return <ErrorState title={t('errorTitle')} error={currentStatus.error} onRetry={() => currentStatus.refetch()} />;
  }

  const demo = currentStatus.data;
  const sizes = demoSizesOf(demo);

  return (
    <Stack gap={5}>
      {demo.clustered && (
        <Alert variant="warning" title={t('state.clusteredTitle')} description={t('state.clusteredBody')} />
      )}
      <Card>
        <CardHeader>
          <Flex align="center" justify="between" gap={3} wrap>
            <CardTitle>{t('title')}</CardTitle>
            <Badge variant={demo.active ? 'warning' : 'secondary'}>
              {demo.active ? t('state.active') : t('state.inactive')}
            </Badge>
          </Flex>
          <TypographyText size="sm" tone="muted">
            {t('intro')}
          </TypographyText>
        </CardHeader>
        <CardContent>
          <Stack gap={5}>
            <DemoStateRows status={demo} />
            <DemoEstateList counts={demo.active ? demo.counts : undefined} config={sizes.initial} />
            <DemoNotes />
            <Flex align="center" gap={2} wrap>
              {!demo.active && <DemoSizesControl mode="enter" sizes={sizes} disabled={!demo.canEnter} />}
              {demo.active && <DemoActionControl action="exit" active variant="outline" />}
              {demo.active && <DemoSizesControl mode="resize" sizes={sizes} variant="outline" />}
              {(demo.active || demo.databaseExists) && (
                <DemoActionControl action="reset" active={demo.active} variant="destructive" />
              )}
            </Flex>
          </Stack>
        </CardContent>
      </Card>
    </Stack>
  );
}

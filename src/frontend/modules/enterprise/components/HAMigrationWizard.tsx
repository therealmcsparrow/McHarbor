// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { type ReactNode, useMemo, useState } from 'react';
import { useFlashValue } from '@resources/hooks/useFlash';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  IconArrowLeft,
  IconArrowRight,
  IconChecklist,
  IconDatabase,
  IconFileText,
  IconRoute,
  IconServer,
} from '@tabler/icons-react';
import { toast } from 'sonner';
import { api } from '@core/api/client';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardHeader, CardTitle } from '@resources/components/ui/Card';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Box, Flex, Grid, Stack } from '@resources/components/ui/Layout';
import { TableSkeleton } from '@resources/components/ui/Skeleton';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { copyToClipboard } from '@resources/utils/clipboard';
import { HAMigrationCommandList, HAMigrationCopySnippet } from './HAMigrationCopySnippet';
import { PERM } from '@core/auth/permissionKeys';
import { assertSuccess } from '@resources/utils/api-mutation';

type MigrationPlan = {
  mode: 'singleton' | 'ha';
  target: 'singleton' | 'ha';
  databaseDriver: string;
  nodeId: string;
  checks: MigrationCheck[];
  resources: MigrationResource[];
  toHA: MigrationStep[];
  toSingleton: MigrationStep[];
  gaps: RoadmapGap[];
};

type MigrationCheck = { id: string; status: 'pass' | 'attention'; required: boolean };
type MigrationStep = { id: string; commands?: string[] };
type MigrationResource = { id: string; value: string; commands?: string[] };
type RoadmapGap = { id: string };
type Direction = 'toHA' | 'toSingleton';

export function HAMigrationWizard() {
  const { t } = useTranslation('enterprise');
  const plan = useQuery({
    queryKey: ['enterprise', 'ha-migration-plan'],
    queryFn: () => api.get<MigrationPlan>('/cluster/ha-migration-plan').then(assertSuccess),
    retry: false,
  });
  const [direction, setDirection] = useState<Direction>('toHA');
  const [index, setIndex] = useState(0);
  const { value: copiedKey, flash: flashCopiedKey } = useFlashValue<string>();
  const [manualCopy, setManualCopy] = useState<{ label: string; value: string } | null>(null);

  const steps = useMemo(() => plan.data?.[direction] ?? [], [direction, plan.data]);
  const current = steps[index];

  if (plan.isLoading) return <TableSkeleton rows={4} columns={3} />;
  if (plan.isError)
    return <ErrorState title={t('haWizard.errorTitle')} error={plan.error} onRetry={() => plan.refetch()} />;
  if (!plan.data || !current) return null;

  const next = () => setIndex((value) => Math.min(value + 1, steps.length - 1));
  const previous = () => setIndex((value) => Math.max(value - 1, 0));
  const chooseDirection = (nextDirection: Direction) => {
    setDirection(nextDirection);
    setIndex(0);
  };
  const copySnippet = async (value: string, key: string, label: string) => {
    const result = await copyToClipboard(value);
    if (!result.ok) {
      setManualCopy({ label, value });
      return;
    }
    flashCopiedKey(key);
    toast.success(t('haWizard.actions.copied'));
  };

  return (
    <>
      <Card>
        <CardHeader>
          <Flex align="center" justify="between" gap={3} wrap>
            <Stack gap={1}>
              <CardTitle>{t('haWizard.title')}</CardTitle>
              <TypographyText tone="muted">{t('haWizard.description')}</TypographyText>
            </Stack>
            <Badge variant={plan.data.mode === 'ha' ? 'success' : 'warning'}>
              {t(`haWizard.mode.${plan.data.mode}`)}
            </Badge>
          </Flex>
        </CardHeader>
        <CardContent className="space-y-5">
          <Grid columns="grid-cols-1 md:grid-cols-3" gap={3}>
            <SummaryTile
              icon={<IconDatabase className="size-5" />}
              label={t('haWizard.summary.database')}
              value={plan.data.databaseDriver}
            />
            <SummaryTile
              icon={<IconServer className="size-5" />}
              label={t('haWizard.summary.node')}
              value={plan.data.nodeId || '-'}
            />
            <SummaryTile
              icon={<IconRoute className="size-5" />}
              label={t('haWizard.summary.target')}
              value={t(`haWizard.mode.${plan.data.target}`)}
            />
          </Grid>

          <Flex gap={2} wrap>
            <Button
              permission={PERM.SYSTEM_MANAGE}
              variant={direction === 'toHA' ? 'default' : 'outline'}
              size="sm"
              onClick={() => chooseDirection('toHA')}
            >
              {t('haWizard.direction.toHA')}
            </Button>
            <Button
              permission={PERM.SYSTEM_MANAGE}
              variant={direction === 'toSingleton' ? 'default' : 'outline'}
              size="sm"
              onClick={() => chooseDirection('toSingleton')}
            >
              {t('haWizard.direction.toSingleton')}
            </Button>
          </Flex>

          <Stack gap={3}>
            <TypographyText weight="semibold">{t('haWizard.resources.title')}</TypographyText>
            <Grid columns="grid-cols-1 md:grid-cols-2" gap={3}>
              {plan.data.resources.map((resource) => (
                <Box key={resource.id} className="rounded-lg border border-border bg-muted/20 p-3">
                  <Flex align="start" gap={3}>
                    <Box className="mc-glass rounded-lg p-2 text-primary">
                      <IconFileText className="size-4" />
                    </Box>
                    <Stack gap={2}>
                      <Stack gap={1}>
                        <TypographyText weight="medium">{t(`haWizard.resources.${resource.id}.title`)}</TypographyText>
                        <TypographyText size="sm" tone="muted">
                          {t(`haWizard.resources.${resource.id}.description`)}
                        </TypographyText>
                        <HAMigrationCopySnippet
                          value={resource.value}
                          copyKey={`resource:${resource.id}`}
                          copiedKey={copiedKey}
                          label={t(`haWizard.resources.${resource.id}.title`)}
                          ariaLabel={t('haWizard.actions.copyValue')}
                          onCopy={copySnippet}
                        />
                      </Stack>
                      {resource.commands?.length ? (
                        <HAMigrationCommandList
                          commands={resource.commands}
                          copiedKey={copiedKey}
                          copyCommandLabel={t('haWizard.actions.copyCommand')}
                          onCopy={copySnippet}
                        />
                      ) : null}
                    </Stack>
                  </Flex>
                </Box>
              ))}
            </Grid>
          </Stack>

          <Grid columns="grid-cols-1 lg:grid-cols-[0.75fr_1.25fr]" gap={4}>
            <Stack gap={3}>
              <TypographyText weight="semibold">{t('haWizard.checks.title')}</TypographyText>
              {plan.data.checks.map((check) => (
                <Flex
                  key={check.id}
                  align="start"
                  justify="between"
                  gap={3}
                  className="rounded-lg border border-border bg-muted/30 p-3"
                >
                  <Stack gap={1}>
                    <TypographyText weight="medium">{t(`haWizard.checks.${check.id}.title`)}</TypographyText>
                    <TypographyText size="sm" tone="muted">
                      {t(`haWizard.checks.${check.id}.detail`)}
                    </TypographyText>
                  </Stack>
                  <Badge variant={check.status === 'pass' ? 'success' : 'warning'}>
                    {t(`haWizard.checkStatus.${check.status}`)}
                  </Badge>
                </Flex>
              ))}
            </Stack>

            <Stack gap={3}>
              <Flex align="center" justify="between" gap={3}>
                <TypographyText weight="semibold">
                  {t('haWizard.steps.progress', { current: index + 1, total: steps.length })}
                </TypographyText>
                <Badge variant="secondary">{t(`haWizard.steps.${current.id}.badge`)}</Badge>
              </Flex>
              <Box className="mc-glass rounded-lg p-4">
                <Stack gap={3}>
                  <Flex align="center" gap={3}>
                    <Box className="rounded-lg border border-border bg-muted/40 p-2">
                      <IconChecklist className="size-5 text-primary" />
                    </Box>
                    <Stack gap={1}>
                      <TypographyText weight="semibold">{t(`haWizard.steps.${current.id}.title`)}</TypographyText>
                      <TypographyText tone="muted">{t(`haWizard.steps.${current.id}.description`)}</TypographyText>
                    </Stack>
                  </Flex>
                  {current.commands?.length ? (
                    <HAMigrationCommandList
                      commands={current.commands}
                      copiedKey={copiedKey}
                      copyCommandLabel={t('haWizard.actions.copyCommand')}
                      onCopy={copySnippet}
                    />
                  ) : null}
                </Stack>
              </Box>
              <Flex justify="between" gap={3}>
                <Button variant="outline" onClick={previous} disabled={index === 0}>
                  <IconArrowLeft className="size-4" />
                  {t('haWizard.actions.previous')}
                </Button>
                <Button onClick={next} disabled={index === steps.length - 1}>
                  {t('haWizard.actions.next')}
                  <IconArrowRight className="size-4" />
                </Button>
              </Flex>
            </Stack>
          </Grid>

          <Stack gap={3}>
            <TypographyText weight="semibold">{t('haWizard.gaps.title')}</TypographyText>
            <Grid columns="grid-cols-1 md:grid-cols-3" gap={3}>
              {plan.data.gaps.map((gap) => (
                <Box key={gap.id} className="rounded-lg border border-dashed border-border bg-muted/20 p-3">
                  <TypographyText weight="medium">{t(`haWizard.gaps.${gap.id}.title`)}</TypographyText>
                  <TypographyText size="sm" tone="muted">
                    {t(`haWizard.gaps.${gap.id}.description`)}
                  </TypographyText>
                </Box>
              ))}
            </Grid>
          </Stack>
        </CardContent>
      </Card>
      <ManualCopyDialog
        open={manualCopy !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setManualCopy(null);
        }}
        value={manualCopy?.value ?? ''}
        title={manualCopy?.label ? `${t('haWizard.actions.manualCopy')}: ${manualCopy.label}` : undefined}
      />
    </>
  );
}

function SummaryTile({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return (
    <Flex align="center" gap={3} className="rounded-lg border border-border bg-muted/30 p-3">
      <Box className="text-primary">{icon}</Box>
      <Stack gap={1}>
        <TypographyText size="sm" tone="muted">
          {label}
        </TypographyText>
        <TypographyText weight="semibold">{value}</TypographyText>
      </Stack>
    </Flex>
  );
}

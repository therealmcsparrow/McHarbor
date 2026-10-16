// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconMinus, IconX, type TablerIcon } from '@tabler/icons-react';
import { Box, Flex, Stack } from '@resources/components/ui/Layout';
import { Spinner } from '@resources/components/ui/Spinner';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';
import type { DatabaseTestResult, DatabaseTestStep } from '../types';

/** The checklist in display order, so a pending run still shows what is coming. */
const STEP_ORDER: DatabaseTestStep['id'][] = [
  'validate',
  'resolve',
  'connect',
  'version',
  'privileges',
  'schema',
  'encoding',
];

const STATUS_ICONS: Record<DatabaseTestStep['status'], TablerIcon> = {
  pass: IconCheck,
  warn: IconAlertTriangle,
  fail: IconX,
  skip: IconMinus,
};

const STATUS_TONES: Record<DatabaseTestStep['status'], string> = {
  pass: 'bg-success/10 text-success',
  warn: 'bg-warning/10 text-warning',
  fail: 'bg-destructive/10 text-destructive',
  skip: 'bg-muted text-muted-foreground',
};

type DatabaseTestStepsProps = {
  result?: DatabaseTestResult | null;
  running?: boolean;
};

/**
 * The seven-step connection check.
 *
 * The whole checklist is always drawn, including the steps that were never
 * reached — a run that stops at `resolve` tells the operator the host is
 * unreachable *and* that the password was never even tried, which is a
 * different conversation from "authentication failed".
 */
export function DatabaseTestSteps({ result, running }: DatabaseTestStepsProps) {
  const { t } = useTranslation('database');
  const byId = new Map((result?.steps ?? []).map((step) => [step.id, step]));

  return (
    <Stack gap={2}>
      {STEP_ORDER.map((id) => {
        const step = byId.get(id);
        const currentStatus = step?.status;
        const Icon = currentStatus ? STATUS_ICONS[currentStatus] : IconMinus;

        return (
          <Flex key={id} align="start" gap={3} className="rounded-lg border border-border bg-card p-4">
            <Box
              className={cn(
                'mt-0.5 flex size-6 shrink-0 items-center justify-center',
                currentStatus ? STATUS_TONES[currentStatus] : 'bg-muted text-muted-foreground',
              )}
            >
              {!step && running ? <Spinner className="size-3" /> : <Icon className="size-4" aria-hidden />}
            </Box>

            <Stack gap={1} className="min-w-0 flex-1">
              <Flex align="center" gap={2} wrap>
                <TypographyText weight="medium">{t(`test.steps.${id}.title`)}</TypographyText>
                {step && (
                  <TypographyText size="xs" tone="muted">
                    {t(`test.status.${step.status}`)}
                    {step.durationMs > 0 ? ` · ${step.durationMs}ms` : ''}
                  </TypographyText>
                )}
              </Flex>
              <TypographyText size="sm" tone="muted">
                {t(`test.steps.${id}.description`)}
              </TypographyText>
              {step?.detail && (
                <TypographyText
                  size="xs"
                  tone={step.status === 'fail' ? 'destructive' : 'muted'}
                  className="font-mono break-all"
                >
                  {step.detail}
                </TypographyText>
              )}
            </Stack>
          </Flex>
        );
      })}
    </Stack>
  );
}

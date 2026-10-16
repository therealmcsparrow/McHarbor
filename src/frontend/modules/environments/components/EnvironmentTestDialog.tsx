// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCircle, IconClock, IconBrandDocker } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import type { EnvironmentTestResult } from '../hooks/useEnvironments';
import { EnvironmentTestSteps } from './EnvironmentTestSteps';

type EnvironmentTestDialogProps = {
  // Result to display. When null the dialog renders an empty
  // loading state (the parent passes the mutation's `data` once
  // it lands). isPending is reflected in a spinner.
  result: EnvironmentTestResult | null | undefined;
  pending: boolean;
  envName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function EnvironmentTestDialog({
  result,
  pending,
  envName,
  open: isOpen,
  onOpenChange,
}: EnvironmentTestDialogProps) {
  const { t } = useTranslation(['environments', 'common']);

  const overall = result?.overall ?? (pending ? 'running' : 'skip');
  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overall === 'pass' ? 'default' : overall === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overall === 'pass'
      ? t('envs.test.resultPass')
      : overall === 'fail'
        ? t('envs.test.resultFail')
        : overall === 'warn'
          ? t('envs.test.resultWarn')
          : overall === 'running'
            ? t('envs.test.running')
            : t('envs.test.resultSkip');

  const OrchestratorIcon = IconBrandDocker;

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <span className="flex items-center gap-2">
              <OrchestratorIcon className="size-4 text-muted-foreground" aria-hidden="true" />
              {t('envs.test.title', { name: envName })}
            </span>
          </DialogTitle>
          <DialogDescription>
            {pending ? t('envs.test.running') : t('envs.test.description', { name: envName })}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="env-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">{t('envs.test.running')}</span>
            </div>
          ) : (
            <div data-testid="env-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    {t('envs.test.overallStatus')}
                  </span>
                  <Badge variant={overallVariant}>{overallLabel}</Badge>
                  {result?.dockerVersion && (
                    <span className="text-xs text-muted-foreground">
                      {t('envs.test.dockerVersion', { version: result.dockerVersion })}
                    </span>
                  )}
                </div>
                {result && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <IconClock className="size-3.5" />
                    {result.duration}
                  </span>
                )}
              </div>

              {result && result.steps.length > 0 ? (
                <EnvironmentTestSteps steps={result.steps} />
              ) : (
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                  <IconCircle className="size-4" aria-hidden="true" />
                  {t('envs.test.noSteps')}
                </div>
              )}
            </div>
          )}
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            {t('close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

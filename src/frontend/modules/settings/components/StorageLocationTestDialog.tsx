// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertCircle, IconAlertTriangle, IconCheck, IconCircle, IconClock, IconHelp } from '@tabler/icons-react';
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
import { cn } from '@resources/utils/cn';
import type { StorageLocationTestResult, StorageLocationTestStep } from '@resources/hooks/useStorageLocations';

type StorageLocationTestDialogProps = {
  // The result to display. When null the dialog renders an empty
  // loading state (the parent passes the mutation's `data` once
  // it lands). isPending is reflected in a spinner.
  result: StorageLocationTestResult | null | undefined;
  pending: boolean;
  locationName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STATUS_ICON: Record<StorageLocationTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<StorageLocationTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<StorageLocationTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

export function StorageLocationTestDialog({
  result,
  pending,
  locationName,
  open: isOpen,
  onOpenChange,
}: StorageLocationTestDialogProps) {
  // The `common` namespace is added so we can resolve i18n keys
  // that live outside the `settings` namespace (e.g. `close`,
  // `actions.save`, etc).
  const { t } = useTranslation(['settings', 'common']);

  const overall = result?.overallStatus ?? (pending ? 'running' : 'skip');
  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overall === 'pass' ? 'default' : overall === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overall === 'pass'
      ? t('storage.test.resultPass')
      : overall === 'fail'
        ? t('storage.test.resultFail')
        : overall === 'warn'
          ? t('storage.test.resultWarn')
          : overall === 'running'
            ? t('storage.test.running')
            : t('storage.test.resultSkip');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('storage.test.title', { name: locationName })}</DialogTitle>
          <DialogDescription>
            {pending ? t('storage.test.running') : t('storage.test.description', { name: locationName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="location-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">{t('storage.test.running')}</span>
            </div>
          ) : (
            <div data-testid="location-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    {t('storage.test.overallStatus')}
                  </span>
                  <Badge variant={overallVariant}>{overallLabel}</Badge>
                </div>
                {result && (
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    <IconClock className="size-3.5" />
                    {result.duration}
                  </span>
                )}
              </div>
              {result && result.steps.length > 0 ? (
                <ul className="space-y-2" data-testid="location-test-steps">
                  {result.steps.map((step, index) => {
                    const StepIcon = STATUS_ICON[step.status];
                    return (
                      <li
                        key={`${step.name}-${index}`}
                        className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
                        data-testid={`storage-test-step-${step.name}`}
                      >
                        <div className="flex items-start gap-3">
                          <StepIcon
                            className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])}
                            aria-hidden="true"
                          />
                          <div className="min-w-0 flex-1 space-y-1">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium text-foreground">
                                {t(`storage.test.steps.${step.name}`, {
                                  // eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: test step names are emitted by the backend and can gain steps without a frontend release
                                  defaultValue: step.name,
                                })}
                              </span>
                              <span className="flex items-center gap-1 text-xs text-muted-foreground">
                                <Badge
                                  variant={
                                    step.status === 'fail'
                                      ? 'destructive'
                                      : step.status === 'warn'
                                        ? 'secondary'
                                        : 'outline'
                                  }
                                >
                                  {t(`storage.test.status.${step.status}`)}
                                </Badge>
                                {step.latency && <span className="font-mono">{step.latency}</span>}
                              </span>
                            </div>
                            {step.detail && <p className="text-xs break-words text-muted-foreground">{step.detail}</p>}
                          </div>
                        </div>
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/20 p-4 text-sm text-muted-foreground">
                  <IconCircle className="size-4" aria-hidden="true" />
                  {t('storage.test.noSteps')}
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

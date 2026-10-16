// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// IdentityProviderTestDialog — the per-step connection-test report for
// an identity provider. Deliberately the same shape, status colours,
// and step-list layout as StorageLocationTestDialog and the AI provider
// equivalent: an operator who has read one of these modals can read all
// of them.
//
// The five steps come from src/backend/modules/identity/test.go —
// load, plugin, configuration, connection, groups. Step details are
// host-authored; the backend never forwards raw upstream error text,
// so nothing here needs escaping beyond normal React rendering.

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
import type { IdentityProviderTestResult, IdentityProviderTestStep } from '../hooks/useIdentityProviders';

type IdentityProviderTestDialogProps = {
  // The report to render. Null while the mutation is in flight — the
  // parent passes the mutation's `data` once it lands.
  result: IdentityProviderTestResult | null | undefined;
  pending: boolean;
  providerName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const STATUS_ICON: Record<IdentityProviderTestStep['status'], typeof IconCheck> = {
  pass: IconCheck,
  fail: IconAlertCircle,
  warn: IconAlertTriangle,
  skip: IconHelp,
};

const STATUS_COLOR: Record<IdentityProviderTestStep['status'], string> = {
  pass: 'text-success',
  fail: 'text-destructive',
  warn: 'text-warning',
  skip: 'text-muted-foreground',
};

const STATUS_BG: Record<IdentityProviderTestStep['status'], string> = {
  pass: 'bg-success/10 border-success/30',
  fail: 'bg-destructive/10 border-destructive/30',
  warn: 'bg-warning/10 border-warning/30',
  skip: 'bg-muted/30 border-border',
};

export function IdentityProviderTestDialog({
  result,
  pending,
  providerName,
  open: isOpen,
  onOpenChange,
}: IdentityProviderTestDialogProps) {
  const { t } = useTranslation(['security', 'common']);

  const overall = result?.overallStatus ?? (pending ? 'running' : 'skip');
  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overall === 'pass' ? 'default' : overall === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overall === 'pass'
      ? t('identity.test.resultPass')
      : overall === 'fail'
        ? t('identity.test.resultFail')
        : overall === 'warn'
          ? t('identity.test.resultWarn')
          : overall === 'running'
            ? t('identity.test.running')
            : t('identity.test.resultSkip');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('identity.test.title', { name: providerName })}</DialogTitle>
          <DialogDescription>
            {pending ? t('identity.test.running') : t('identity.test.description', { name: providerName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="idp-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">{t('identity.test.running')}</span>
            </div>
          ) : (
            <div data-testid="idp-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    {t('identity.test.overallStatus')}
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
                <ul className="space-y-2" data-testid="idp-test-steps">
                  {result.steps.map((step) => {
                    const StepIcon = STATUS_ICON[step.status];
                    return (
                      <li
                        key={step.name}
                        className={cn('rounded-lg border p-3 text-sm', STATUS_BG[step.status])}
                        data-testid={`idp-test-step-${step.name}`}
                      >
                        <div className="flex items-start gap-3">
                          <StepIcon
                            className={cn('mt-0.5 size-4 shrink-0', STATUS_COLOR[step.status])}
                            aria-hidden="true"
                          />
                          <div className="min-w-0 flex-1 space-y-1">
                            <div className="flex items-center justify-between gap-2">
                              <span className="font-medium text-foreground">
                                {/* eslint-disable-next-line mcharbor/no-restricted-syntax -- i18n-open-set: test step names are emitted by the backend and can gain steps without a frontend release */}
                                {t(`identity.test.steps.${step.name}`, { defaultValue: step.name })}
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
                                  {t(`identity.test.status.${step.status}`)}
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
                  {t('identity.test.noSteps')}
                </div>
              )}
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            {t('actions.close', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

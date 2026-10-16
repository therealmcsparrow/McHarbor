// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The scanner test report.
//
// Same layout, status colours and step list as StorageLocationTestDialog
// and TestChannelDialog — three families, one visual language, so an
// operator who has read one of these modals can read all of them.
//
// Steps come from src/backend/modules/scans/test.go (load, config,
// plugin, availability) followed by whatever the adapter itself reports
// via RicherScannerTester — CLI engines report binary / version /
// database, service engines report configuration / indexer / matcher.

import { useTranslation } from 'react-i18next';
import { IconClock } from '@tabler/icons-react';
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
import type { ScannerTestResult } from '../hooks/useScanners';
import { ScannerTestStepList } from './ScannerTestStepList';

type ScannerTestDialogProps = {
  /**
   * The report to display. Null while the request is in flight — the
   * parent passes the mutation's `data` once it lands.
   */
  result: ScannerTestResult | null | undefined;
  pending: boolean;
  scannerName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ScannerTestDialog({
  result,
  pending,
  scannerName,
  open: isOpen,
  onOpenChange,
}: ScannerTestDialogProps) {
  // The `common` namespace is added so keys outside `security`
  // (e.g. `close`) resolve.
  const { t } = useTranslation(['security', 'common']);

  const overall = result?.overallStatus ?? (pending ? 'running' : 'skip');
  const overallVariant: 'default' | 'destructive' | 'secondary' =
    overall === 'pass' ? 'default' : overall === 'fail' ? 'destructive' : 'secondary';

  const overallLabel: string =
    overall === 'pass'
      ? t('scanning.test.resultPass')
      : overall === 'fail'
        ? t('scanning.test.resultFail')
        : overall === 'warn'
          ? t('scanning.test.resultWarn')
          : overall === 'running'
            ? t('scanning.test.running')
            : t('scanning.test.resultSkip');

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('scanning.test.title', { name: scannerName })}</DialogTitle>
          <DialogDescription>
            {pending ? t('scanning.test.running') : t('scanning.test.description', { name: scannerName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {pending ? (
            <div
              className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-4"
              data-testid="scan-test-running"
            >
              <Spinner size="md" />
              <span className="text-sm text-foreground">{t('scanning.test.running')}</span>
            </div>
          ) : (
            <div data-testid="scan-test-result" className="space-y-3">
              <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                    {t('scanning.test.overallStatus')}
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
              <ScannerTestStepList steps={result?.steps ?? []} />
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

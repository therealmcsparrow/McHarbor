// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Spinner } from '@resources/components/ui/Spinner';
import type { DirectTransferTestResult } from '../types';
import { AgentDirectTransferTestResult } from './AgentDirectTransferTestResult';

type AgentDirectTransferTestDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  result: DirectTransferTestResult | null;
  requestError: string;
};

export function AgentDirectTransferTestDialog({
  open,
  onOpenChange,
  pending,
  result,
  requestError,
}: AgentDirectTransferTestDialogProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('agent.directTransferTest.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('agent.directTransferTest.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody>
          {pending ? (
            <div className="flex min-h-40 flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
              <Spinner />
              <span>{t('agent.directTransferTest.testing')}</span>
            </div>
          ) : requestError ? (
            <div className="rounded-lg border border-border bg-background/40 p-4 text-sm text-foreground">
              <Badge variant="destructive">{t('agent.directTransferTest.failed')}</Badge>
              <p className="mt-3 break-words text-muted-foreground">{requestError}</p>
            </div>
          ) : result ? (
            <AgentDirectTransferTestResult result={result} />
          ) : null}
        </DialogBody>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">{tc('actions.close')}</Button>
          </DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

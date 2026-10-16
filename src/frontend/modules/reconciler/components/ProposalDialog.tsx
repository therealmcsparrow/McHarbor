// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Textarea } from '@resources/components/ui/Textarea';
import { Label } from '@resources/components/ui/Label';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import { type Proposal, useApproveProposal, useRejectProposal } from '../hooks/useProposals';
import { reasonOf } from '../hooks/refusal';
import { PlanSteps } from './PlanSteps';

type ProposalDialogProps = {
  proposal: Proposal;
  onOpenChange: (open: boolean) => void;
};

export function ProposalDialog({ proposal, onOpenChange }: ProposalDialogProps) {
  const { t } = useTranslation('reconciler');
  const [shown, setShown] = useState(proposal);
  const [confirming, setConfirming] = useState<'approve' | 'reject' | null>(null);
  const [note, setNote] = useState('');
  const approve = useApproveProposal();
  const reject = useRejectProposal();
  const pending = approve.isPending || reject.isPending;
  const error = approve.error ?? reject.error;
  const reason = reasonOf(error);
  const [staleNotice, setStaleNotice] = useState(false);
  const isOpen = shown.status === 'pending';

  const runApprove = () => {
    if (pending) return;
    approve.mutate(shown.id, {
      onSuccess: (outcome) => {
        setConfirming(null);
        setStaleNotice(outcome.stale);
        setShown(outcome.proposal);
      },
    });
  };
  const runReject = () => {
    if (pending) return;
    reject.mutate(
      { id: shown.id, note },
      {
        onSuccess: (p) => {
          setConfirming(null);
          setShown(p);
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('proposals.dialogTitle', { name: shown.desiredName })}</DialogTitle>
          <DialogDescription>
            {t('proposals.dialogDescription', { time: formatDate(shown.createdAt) })}{' '}
            <Badge variant="secondary">{t(`proposals.status.${shown.status}`)}</Badge>
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto">
          {staleNotice && <Alert variant="warning">{t('proposals.staleNotice')}</Alert>}
          <section>
            <h4 className="mb-2 text-sm font-semibold text-foreground">{t('proposals.drift')}</h4>
            <ul className="space-y-1 text-xs">
              {(shown.report?.diffs ?? []).map((d) => (
                <li key={`${d.class}|${d.field}`} className="font-mono break-all">
                  <Badge variant="warning" className="mr-2">
                    {t(`class.${d.class}`)}
                  </Badge>
                  {d.field}: {d.expected} → {d.actual}
                </li>
              ))}
            </ul>
          </section>
          <section>
            <h4 className="mb-2 text-sm font-semibold text-foreground">{t('proposals.plan')}</h4>
            <PlanSteps steps={shown.steps} results={shown.results} />
          </section>
          {shown.error && <Alert variant="destructive">{shown.error}</Alert>}
          {confirming === 'approve' && <Alert variant="warning">{t('proposals.approveWarning')}</Alert>}
          {confirming === 'reject' && (
            <div>
              <Label htmlFor="reject-note" className="mb-2">
                {t('proposals.rejectNote')}
              </Label>
              <Textarea id="reject-note" value={note} maxLength={1000} onChange={(e) => setNote(e.target.value)} />
            </div>
          )}
          {error && <Alert variant="destructive">{reason ? t(`refusal.${reason}`) : error.message}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => (confirming ? setConfirming(null) : onOpenChange(false))}>
            {confirming ? t('actions.back', { ns: 'common' }) : t('actions.close', { ns: 'common' })}
          </Button>
          {isOpen && confirming === null && (
            <>
              <Button variant="outline" permission={PERM.RECONCILER_MANAGE} onClick={() => setConfirming('reject')}>
                {t('proposals.reject')}
              </Button>
              <Button permission={PERM.RECONCILER_MANAGE} onClick={() => setConfirming('approve')}>
                {t('proposals.approve')}
              </Button>
            </>
          )}
          {isOpen && confirming === 'approve' && (
            <Button variant="destructive" permission={PERM.RECONCILER_MANAGE} disabled={pending} onClick={runApprove}>
              {approve.isPending ? t('actions.processing', { ns: 'common' }) : t('proposals.confirmApprove')}
            </Button>
          )}
          {isOpen && confirming === 'reject' && (
            <Button variant="destructive" permission={PERM.RECONCILER_MANAGE} disabled={pending} onClick={runReject}>
              {reject.isPending ? t('actions.processing', { ns: 'common' }) : t('proposals.confirmReject')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

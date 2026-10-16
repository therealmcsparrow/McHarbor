// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { PERM } from '@core/auth/permissionKeys';
import type { DesiredState } from '../hooks/useReconciler';
import { usePullRequest } from '../hooks/useSources';
import { reasonOf } from '../hooks/refusal';

type PullRequestDialogProps = {
  state: DesiredState;
  onOpenChange: (open: boolean) => void;
};

export function PullRequestDialog({ state, onOpenChange }: PullRequestDialogProps) {
  const { t } = useTranslation('reconciler');
  const pr = usePullRequest();
  const result = pr.data;
  const reason = reasonOf(pr.error);

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('pullRequest.title', { name: state.name })}</DialogTitle>
          <DialogDescription>{t('pullRequest.description')}</DialogDescription>
        </DialogHeader>
        {result ? (
          <div className="space-y-2 text-sm">
            <p>
              {t('pullRequest.branch')} <span className="font-mono">{result.branch}</span>
            </p>
            {result.pullRequestUrl ? (
              <a className="text-primary underline" href={result.pullRequestUrl} target="_blank" rel="noreferrer">
                {t('pullRequest.open')}
              </a>
            ) : (
              <Alert variant="warning">{t('pullRequest.noRequest', { error: result.providerError ?? '' })}</Alert>
            )}
            <p className="font-mono text-xs break-all">
              {t('pullRequest.changed', { fields: result.changed.join(', ') })}
            </p>
            {result.skipped.length > 0 && (
              <p className="font-mono text-xs break-all text-muted-foreground">
                {t('pullRequest.skipped', { fields: result.skipped.join(', ') })}
              </p>
            )}
          </div>
        ) : (
          pr.error && <Alert variant="destructive">{reason ? t(`refusal.${reason}`) : pr.error.message}</Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.close', { ns: 'common' })}
          </Button>
          {!result && (
            <Button
              permission={PERM.RECONCILER_MANAGE}
              disabled={pr.isPending}
              onClick={() => !pr.isPending && pr.mutate(state.id)}
            >
              {pr.isPending ? t('actions.processing', { ns: 'common' }) : t('pullRequest.confirm')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

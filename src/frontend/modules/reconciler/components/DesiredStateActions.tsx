// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import type { UseMutationResult } from '@tanstack/react-query';
import { IconAdjustments, IconGitPullRequest, IconRefresh, IconSearch, IconTrash, IconWand } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import type { DesiredState } from '../hooks/useReconciler';

type DesiredStateActionsProps = {
  state: DesiredState;
  reconcile: UseMutationResult<unknown, Error, string>;
  checkDrift: UseMutationResult<unknown, Error, string>;
  onDelete: (id: string) => void;
  onMode: (state: DesiredState) => void;
  onPropose: (state: DesiredState) => void;
  onPullRequest: (state: DesiredState) => void;
};

export function DesiredStateActions({
  state,
  reconcile,
  checkDrift,
  onDelete,
  onMode,
  onPropose,
  onPullRequest,
}: DesiredStateActionsProps) {
  const { t } = useTranslation('common');
  const observeOnly = state.kind !== 'container';
  const reconcileLabel = observeOnly ? t('observeOnly', { ns: 'reconciler' }) : t('reconciler.reconcileTooltip');
  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        permission={PERM.RECONCILER_MANAGE}
        variant="ghost"
        size="icon"
        disabled={observeOnly}
        title={reconcileLabel}
        aria-label={reconcileLabel}
        onClick={(evt2) => {
          evt2.stopPropagation();
          reconcile.mutate(state.id);
        }}
      >
        <IconRefresh className="h-4 w-4" />
      </Button>
      <Button
        permission={PERM.RECONCILER_MANAGE}
        variant="ghost"
        size="icon"
        title={t('reconciler.checkDriftTooltip')}
        aria-label={t('reconciler.checkDriftTooltip')}
        onClick={(evt2) => {
          evt2.stopPropagation();
          checkDrift.mutate(state.id);
        }}
      >
        <IconSearch className="h-4 w-4" />
      </Button>
      <Button
        permission={PERM.RECONCILER_MANAGE}
        variant="ghost"
        size="icon"
        title={t('mode.change', { ns: 'reconciler' })}
        aria-label={t('mode.change', { ns: 'reconciler' })}
        onClick={(evt2) => {
          evt2.stopPropagation();
          onMode(state);
        }}
      >
        <IconAdjustments className="h-4 w-4" />
      </Button>
      <Button
        permission={PERM.RECONCILER_MANAGE}
        variant="ghost"
        size="icon"
        disabled={!state.driftDetected}
        title={t('proposals.proposeNow', { ns: 'reconciler' })}
        aria-label={t('proposals.proposeNow', { ns: 'reconciler' })}
        onClick={(entry) => {
          entry.stopPropagation();
          onPropose(state);
        }}
      >
        <IconWand className="h-4 w-4" />
      </Button>
      {state.sourceId && (
        <Button
          permission={PERM.RECONCILER_MANAGE}
          variant="ghost"
          size="icon"
          disabled={!state.driftDetected}
          title={t('pullRequest.action', { ns: 'reconciler' })}
          aria-label={t('pullRequest.action', { ns: 'reconciler' })}
          onClick={(eventItem) => {
            eventItem.stopPropagation();
            onPullRequest(state);
          }}
        >
          <IconGitPullRequest className="h-4 w-4" />
        </Button>
      )}
      <Button
        permission={PERM.RECONCILER_MANAGE}
        variant="ghost"
        size="icon"
        disabled={Boolean(state.sourceId)}
        title={state.sourceId ? t('gitManagedDelete', { ns: 'reconciler' }) : t('reconciler.deleteTooltip')}
        aria-label={t('reconciler.deleteTooltip')}
        onClick={(evt) => {
          evt.stopPropagation();
          onDelete(state.id);
        }}
      >
        <IconTrash className="h-4 w-4 text-destructive" />
      </Button>
    </div>
  );
}

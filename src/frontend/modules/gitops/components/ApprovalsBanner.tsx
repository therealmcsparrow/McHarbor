// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import type { useGitOpsApprovals, useResolveApproval } from '../hooks/useGitOps';

type ApprovalsBannerProps = {
  approvals: NonNullable<ReturnType<typeof useGitOpsApprovals>['data']>;
  resolve: ReturnType<typeof useResolveApproval>;
};

export function ApprovalsBanner({ approvals, resolve }: ApprovalsBannerProps) {
  const { t } = useTranslation('gitops');
  return (
    <div className="rounded-lg border border-warning/40 bg-warning/10 p-4">
      <h3 className="mb-2 text-sm font-semibold text-foreground">{t('approvals')}</h3>
      <div className="space-y-2">
        {approvals.map((a) => (
          <div key={a.id} className="mc-glass flex items-center justify-between rounded-md px-3 py-2 text-sm">
            <div className="min-w-0 flex-1">
              <div className="truncate font-medium">
                {a.pipelineId} → {a.stageId}
              </div>
              <div className="text-xs text-muted-foreground">
                {a.commitSha} · {a.requestedBy} · {a.requestedAt}
              </div>
            </div>
            <div className="flex gap-2">
              <Button
                permission={PERM.GITOPS_APPROVE}
                size="sm"
                onClick={() =>
                  resolve.mutate({
                    id: a.id,
                    action: 'approve',
                    input: { note: '', resolvedBy: '' },
                  })
                }
                disabled={resolve.isPending}
              >
                {t('approve')}
              </Button>
              <Button
                permission={PERM.GITOPS_APPROVE}
                size="sm"
                variant="outline"
                onClick={() =>
                  resolve.mutate({
                    id: a.id,
                    action: 'reject',
                    input: { note: '', resolvedBy: '' },
                  })
                }
                disabled={resolve.isPending}
              >
                {t('reject')}
              </Button>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

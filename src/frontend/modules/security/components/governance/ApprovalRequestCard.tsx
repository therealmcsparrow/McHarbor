// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { StatusError } from '@resources/components/ui/StatusError';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { ApprovalRequest, ApprovalStatus } from '../../hooks/useApprovals';

type Variant = 'default' | 'secondary' | 'success' | 'warning' | 'destructive' | 'info';

const STATUS_VARIANT: Record<ApprovalStatus, Variant> = {
  pending: 'warning',
  approved: 'info',
  denied: 'destructive',
  expired: 'secondary',
  cancelled: 'secondary',
  executing: 'info',
  executed: 'success',
};

type ApprovalRequestCardProps = {
  request: ApprovalRequest;
  currentUserId: string | undefined;
  onDecide: (request: ApprovalRequest, approve: boolean) => void;
  onExecute: (request: ApprovalRequest) => void;
  onCancel: (request: ApprovalRequest) => void;
};

export function ApprovalRequestCard({
  request,
  currentUserId,
  onDecide,
  onExecute,
  onCancel,
}: ApprovalRequestCardProps) {
  const { t } = useTranslation('security');
  const mine = request.requesterId === currentUserId;
  const isOpen = request.status === 'pending' || request.status === 'approved';
  const failed = request.status === 'executed' && (request.resultStatus ?? 0) >= 400;

  return (
    <li className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm font-semibold text-foreground">
              {t(`governance.categories.${request.category}`)}
            </span>
            <Badge variant={STATUS_VARIANT[request.status]}>{t(`governance.status.${request.status}`)}</Badge>
            <StatusError
              error={
                failed
                  ? t('governance.approvals.resultFailed', {
                      status: request.resultStatus,
                      code: request.resultCode || '-',
                    })
                  : null
              }
            />
          </div>
          <TypographyText size="xs" tone="muted">
            {t('governance.approvals.summary', {
              requester: request.requesterName,
              resource: request.resourceName || '-',
              count: request.approvalsCount,
              required: request.requiredApprovers,
            })}
          </TypographyText>
          <TypographyText size="xs" tone="muted" className="font-mono break-all">
            {request.method} {request.path}
          </TypographyText>
          <TypographyText size="xs" tone="muted">
            {isOpen
              ? t('governance.approvals.expires', { date: formatDate(request.expiresAt) })
              : t('governance.approvals.created', { date: formatDate(request.createdAt) })}
          </TypographyText>
        </div>
        <div className="flex flex-wrap gap-2">
          {request.status === 'pending' && !mine && (
            <>
              <Button
                size="sm"
                variant="outline"
                permission={PERM.APPROVALS_DECIDE}
                onClick={() => onDecide(request, false)}
              >
                {t('governance.approvals.deny')}
              </Button>
              <Button size="sm" permission={PERM.APPROVALS_DECIDE} onClick={() => onDecide(request, true)}>
                {t('governance.approvals.approve')}
              </Button>
            </>
          )}
          {mine && request.status === 'approved' && (
            <Button size="sm" onClick={() => onExecute(request)}>
              {t('governance.approvals.execute')}
            </Button>
          )}
          {mine && isOpen && (
            <Button size="sm" variant="ghost" onClick={() => onCancel(request)}>
              {t('governance.approvals.cancel')}
            </Button>
          )}
        </div>
      </div>
      {request.bodyRedacted && (
        <pre className="mt-3 max-h-40 overflow-auto rounded-md bg-muted p-2 text-xs text-foreground">
          {request.bodyRedacted}
        </pre>
      )}
      {request.decisions.length > 0 && (
        <ul className="mt-3 space-y-1">
          {request.decisions.map((d) => (
            <li key={d.id} className="text-xs text-muted-foreground">
              {t(
                d.decision === 'approve' ? 'governance.approvals.decisionApprove' : 'governance.approvals.decisionDeny',
                {
                  name: d.approverName,
                  date: formatDate(d.createdAt),
                },
              )}
              {d.note && (
                <span className="ml-1 text-foreground">{t('governance.approvals.noteQuoted', { note: d.note })}</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

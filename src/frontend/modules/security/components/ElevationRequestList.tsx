// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCheck, IconX, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { formatDate } from '@resources/utils/format';
import { PERM } from '@core/auth/permissionKeys';
import type { ElevationRequest } from '@resources/hooks/useElevations';

function StatusBadge({ status: statusValue }: { status: ElevationRequest['status'] }) {
  const { t } = useTranslation('security');
  const variant = statusValue === 'approved' ? 'success' : statusValue === 'pending' ? 'secondary' : 'warning';
  return <Badge variant={variant}>{t(`elevations.status.${statusValue}`)}</Badge>;
}

type ElevationRequestListProps = {
  requests: ElevationRequest[];
  loading: boolean;
  emptyLabel: string;
  /** Approver view: renders approve/deny instead of the status badge. */
  onDecide?: (id: string, approve: boolean) => void;
  onWithdraw?: (id: string) => void;
  busy?: boolean;
};

/**
 * One row per elevation request, in either of the two roles this tab shows:
 * the requester's own list (status + withdraw) and the approver inbox
 * (approve / deny). Extracted from `ElevationsTab` to keep that component
 * under the 200-line limit.
 */
export function ElevationRequestList({
  requests,
  loading,
  emptyLabel,
  onDecide,
  onWithdraw,
  busy,
}: ElevationRequestListProps) {
  const { t } = useTranslation('security');

  if (loading) {
    return (
      <div className="flex justify-center py-6">
        <Spinner />
      </div>
    );
  }
  if (requests.length === 0) {
    return <p className="py-4 text-center text-sm text-muted-foreground">{emptyLabel}</p>;
  }

  return (
    <div className="space-y-2">
      {requests.map((req) => (
        <div
          key={req.id}
          className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-background/40 p-3"
        >
          <div className="min-w-0">
            <p className="truncate text-sm text-foreground">
              {req.permissionLabel || req.permissionKey}
              {req.permissionLabel && (
                <span className="ml-2 font-mono text-xs text-muted-foreground">{req.permissionKey}</span>
              )}
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {onDecide
                ? `${t('elevations.requestedFor', { minutes: req.requestedMinutes })} · ${req.justification}`
                : req.justification}
            </p>
          </div>
          <div className="flex items-center gap-1">
            {onDecide ? (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  permission={PERM.USERS_MANAGE}
                  disabled={busy}
                  onClick={() => onDecide(req.id, true)}
                >
                  <IconCheck className="size-4" aria-hidden="true" />
                  {t('elevations.approve')}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  permission={PERM.USERS_MANAGE}
                  disabled={busy}
                  onClick={() => onDecide(req.id, false)}
                >
                  <IconX className="size-4" aria-hidden="true" />
                  {t('elevations.deny')}
                </Button>
              </>
            ) : (
              <>
                {req.approvedUntil && req.status === 'approved' && (
                  <span className="text-xs text-muted-foreground">
                    {t('elevations.until', { when: formatDate(req.approvedUntil) })}
                  </span>
                )}
                <StatusBadge status={req.status} />
                {req.status === 'pending' && onWithdraw && (
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={t('elevations.withdraw')}
                    onClick={() => onWithdraw(req.id)}
                  >
                    <IconTrash className="size-4" aria-hidden="true" />
                  </Button>
                )}
              </>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}

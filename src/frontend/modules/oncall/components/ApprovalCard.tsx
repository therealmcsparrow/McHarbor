// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { formatDateCustom } from '@resources/utils/format';
import { IconCheck, IconX } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Button } from '@resources/components/ui/Button';
import type { ElevationRequest } from '@resources/hooks/useElevations';
import { useOnline } from '@resources/hooks/useOnline';
import { DecisionDialog } from './DecisionDialog';

type ApprovalCardProps = {
  request: ElevationRequest;
  requester: string;
  environment?: string;
};

export function ApprovalCard({ request, requester, environment }: ApprovalCardProps) {
  const { t } = useTranslation('oncall');
  const online = useOnline();
  const [decision, setDecision] = useState<boolean | null>(null);
  const whenValue = formatDateCustom(request.createdAt, { dateStyle: 'short', timeStyle: 'short' });

  return (
    <li className="rounded-lg border border-border bg-card p-4">
      <div className="flex items-baseline gap-2">
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-semibold">{request.permissionLabel || request.permissionKey}</p>
          {request.permissionLabel && (
            <code className="block truncate font-mono text-xs text-muted-foreground">{request.permissionKey}</code>
          )}
        </div>
        <time dateTime={request.createdAt} className="text-xs text-muted-foreground">
          {whenValue}
        </time>
      </div>
      <p className="mt-1 text-sm">{t('approvals.requestedBy', { name: requester })}</p>
      <p className="text-sm text-muted-foreground">
        {t('approvals.duration', { count: request.requestedMinutes })}
        {environment && ` · ${t('approvals.environment', { name: environment })}`}
      </p>
      {request.justification && (
        <p className="mt-2 rounded-md bg-muted/40 p-2 text-sm break-words whitespace-pre-wrap">
          {request.justification}
        </p>
      )}
      <div className="mt-3 grid grid-cols-2 gap-2">
        <Button
          variant="outline"
          className="min-h-12"
          permission={PERM.USERS_MANAGE}
          disabled={!online}
          onClick={() => setDecision(false)}
        >
          <IconX className="size-5" aria-hidden="true" />
          {t('approvals.deny')}
        </Button>
        <Button
          className="min-h-12"
          permission={PERM.USERS_MANAGE}
          disabled={!online}
          onClick={() => setDecision(true)}
        >
          <IconCheck className="size-5" aria-hidden="true" />
          {t('approvals.approve')}
        </Button>
      </div>
      <DecisionDialog
        request={request}
        requester={requester}
        approve={decision ?? false}
        open={decision !== null}
        onOpenChange={(isOpen) => {
          if (!isOpen) setDecision(null);
        }}
      />
    </li>
  );
}

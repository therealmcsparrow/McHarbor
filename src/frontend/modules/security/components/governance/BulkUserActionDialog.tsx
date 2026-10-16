// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Input } from '@resources/components/ui/Input';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { PERM } from '@core/auth/permissionKeys';
import { useBulkUserAction, type BulkAction, type BulkItem } from '../../hooks/useUserLifecycle';

type BulkUserActionDialogProps = {
  action: BulkAction | null;
  userIds: string[];
  onOpenChange: (open: boolean) => void;
  onDone?: () => void;
};

const OUTCOME_VARIANT: Record<BulkItem['outcome'], 'success' | 'secondary' | 'destructive' | 'info'> = {
  would_apply: 'info',
  applied: 'success',
  skipped: 'secondary',
  failed: 'destructive',
};

const REASONS = [
  'self',
  'not_found',
  'last_admin',
  'already_disabled',
  'not_disabled',
  'not_locked',
  'idp_managed',
  'error',
];

export function BulkUserActionDialog({ action, userIds, onOpenChange, onDone }: BulkUserActionDialogProps) {
  const { t } = useTranslation('security');
  const preview = useBulkUserAction();
  const apply = useBulkUserAction();
  const [reason, setReason] = useState('');
  const isOpen = action !== null;
  const { mutate: runPreview, reset: resetPreview } = preview;
  const idsKey = userIds.join(',');

  useEffect(() => {
    if (!action) {
      resetPreview();
      return;
    }
    runPreview({ action, userIds: idsKey.split(','), dryRun: true, reason: '' });
  }, [action, idsKey, runPreview, resetPreview]);

  const willApply = preview.data?.summary.applied ?? 0;

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={(next) => {
        if (!next) setReason('');
        onOpenChange(next);
      }}
      title={action ? t(`governance.lifecycle.actions.${action}Title`, { count: userIds.length }) : ''}
      description={t('governance.lifecycle.previewDescription')}
      confirmLabel={action ? t(`governance.lifecycle.actions.${action}`) : ''}
      variant={action === 'enable' || action === 'unlock' ? 'caution' : 'destructive'}
      permission={action === 'delete' ? PERM.USERS_MANAGE : PERM.ACCOUNT_MANAGE}
      mutation={apply}
      variables={{ action: action ?? 'disable', userIds, dryRun: false, reason: reason.trim() }}
      disabled={!preview.data || willApply === 0}
      onSuccess={onDone}
    >
      {action === 'disable' && (
        <Input
          aria-label={t('governance.lifecycle.reason')}
          placeholder={t('governance.lifecycle.reason')}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      )}
      {preview.isPending && (
        <TypographyText size="xs" tone="muted">
          {t('governance.lifecycle.previewing')}
        </TypographyText>
      )}
      {preview.isError && (
        <p role="alert" className="text-xs text-destructive">
          {preview.error.message}
        </p>
      )}
      {preview.data && (
        <div className="space-y-2">
          <TypographyText size="xs">{t('governance.lifecycle.previewSummary', preview.data.summary)}</TypographyText>
          <ul className="max-h-56 space-y-1 overflow-auto">
            {preview.data.items.map((item) => (
              <li key={item.userId} className="flex items-center justify-between gap-2 text-xs">
                <span className="truncate text-foreground">{item.username || item.userId}</span>
                <Badge variant={OUTCOME_VARIANT[item.outcome]}>
                  {item.reason && REASONS.includes(item.reason)
                    ? t(`governance.lifecycle.reasons.${item.reason}`)
                    : t(`governance.lifecycle.outcomes.${item.outcome}`)}
                </Badge>
              </li>
            ))}
          </ul>
        </div>
      )}
    </MutationConfirmDialog>
  );
}

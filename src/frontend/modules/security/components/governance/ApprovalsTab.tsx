// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconChecklist } from '@tabler/icons-react';
import { SettingsGroup } from '@resources/components/settings/SettingsGroup';
import { Select } from '@resources/components/ui/Select';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useAuth } from '@core/auth/useAuth';
import { usePermission } from '@core/auth/usePermission';
import {
  useApprovalRequests,
  useCancelApproval,
  useExecuteApproval,
  type ApprovalRequest,
  type ApprovalScope,
} from '../../hooks/useApprovals';
import { ApprovalDecisionDialog } from './ApprovalDecisionDialog';
import { ApprovalRequestCard } from './ApprovalRequestCard';

const STATUSES = ['pending', 'approved', 'denied', 'expired', 'executed', 'cancelled'] as const;

export function ApprovalsTab() {
  const { t } = useTranslation('security');
  const myId = useAuth((s) => s.user?.id);
  const canSeeAll = usePermission([PERM.APPROVALS_VIEW, PERM.APPROVALS_DECIDE]);
  const [scope, setScope] = useState<ApprovalScope>(canSeeAll ? 'all' : 'mine');
  const [statusValue, setStatus] = useState<string>('pending');
  const effectiveScope: ApprovalScope = canSeeAll ? scope : 'mine';
  const requests = useApprovalRequests(effectiveScope, statusValue === 'any' ? '' : statusValue);

  const [deciding, setDeciding] = useState<{ request: ApprovalRequest; approve: boolean } | null>(null);
  const [executing, setExecuting] = useState<ApprovalRequest | null>(null);
  const [cancelling, setCancelling] = useState<ApprovalRequest | null>(null);
  const execute = useExecuteApproval();
  const cancel = useCancelApproval();

  const scopeOptions = [
    { value: 'all', label: t('governance.approvals.scopeAll') },
    { value: 'mine', label: t('governance.approvals.scopeMine') },
  ];
  const statusOptions = [
    { value: 'any', label: t('governance.status.any') },
    ...STATUSES.map((s) => ({ value: s, label: t(`governance.status.${s}`) })),
  ];

  return (
    <SettingsGroup
      icon={IconChecklist}
      title={t('governance.approvals.title')}
      description={t('governance.approvals.description')}
    >
      <div className="flex flex-wrap gap-3">
        {canSeeAll && (
          <div className="w-48">
            <Select
              ariaLabel={t('governance.approvals.scope')}
              value={effectiveScope}
              onChange={(v) => setScope(v as ApprovalScope)}
              options={scopeOptions}
              searchable={false}
            />
          </div>
        )}
        <div className="w-48">
          <Select
            ariaLabel={t('governance.approvals.statusFilter')}
            value={statusValue}
            onChange={setStatus}
            options={statusOptions}
            searchable={false}
          />
        </div>
      </div>

      {requests.isLoading && (
        <TypographyText size="sm" tone="muted">
          {t('common:common.loading')}
        </TypographyText>
      )}
      {requests.data && requests.data.length === 0 && (
        <TypographyText size="sm" tone="muted">
          {t('governance.approvals.empty')}
        </TypographyText>
      )}
      <ul className="space-y-3">
        {(requests.data ?? []).map((r) => (
          <ApprovalRequestCard
            key={r.id}
            request={r}
            currentUserId={myId}
            onDecide={(request, approve) => setDeciding({ request, approve })}
            onExecute={setExecuting}
            onCancel={setCancelling}
          />
        ))}
      </ul>

      <ApprovalDecisionDialog
        request={deciding?.request ?? null}
        approve={deciding?.approve ?? true}
        onOpenChange={(dialogOpen) => !dialogOpen && setDeciding(null)}
      />
      <MutationConfirmDialog
        open={executing !== null}
        onOpenChange={(openState) => !openState && setExecuting(null)}
        title={t('governance.approvals.executeTitle')}
        description={t('governance.approvals.executeDescription', {
          category: executing ? t(`governance.categories.${executing.category}`) : '',
          resource: executing?.resourceName || executing?.path || '',
        })}
        confirmLabel={t('governance.approvals.execute')}
        mutation={execute}
        variables={executing?.id ?? ''}
      />
      <MutationConfirmDialog
        open={cancelling !== null}
        onOpenChange={(isOpen) => !isOpen && setCancelling(null)}
        title={t('governance.approvals.cancelTitle')}
        description={t('governance.approvals.cancelDescription')}
        confirmLabel={t('governance.approvals.cancel')}
        variant="caution"
        mutation={cancel}
        variables={cancelling?.id ?? ''}
      />
    </SettingsGroup>
  );
}

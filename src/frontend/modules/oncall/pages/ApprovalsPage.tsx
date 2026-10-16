// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PERM } from '@core/auth/permissionKeys';
import { useAuth } from '@core/auth/useAuth';
import { usePermission } from '@core/auth/usePermission';
import { useEnvironmentList } from '@resources/hooks/useEnvironmentList';
import type { ElevationRequest } from '@resources/hooks/useElevations';
import { useUsers } from '@resources/hooks/useUsers';
import { ApprovalCard } from '../components/ApprovalCard';
import { QueryState } from '../components/QueryState';
import { StaleNotice } from '@resources/components/StaleNotice';
import { useOnCallQuery } from '@resources/hooks/useOnCallQuery';

export default function ApprovalsPage() {
  const { t } = useTranslation('oncall');
  const canDecide = usePermission(PERM.USERS_MANAGE);
  const pending = useOnCallQuery<{ requests: ElevationRequest[] }>(
    ['elevations', 'pending'],
    '/elevations/pending',
    undefined,
    {
      enabled: canDecide,
      refetchInterval: 30_000,
    },
  );
  const users = useUsers();
  const environments = useEnvironmentList();
  const userNames = useMemo(
    () => new Map((users.data ?? []).map((user): [string, string] => [user.id, user.displayName || user.username])),
    [users.data],
  );
  const envNames = useMemo(
    () => new Map((environments.data ?? []).map((env) => [env.id, env.name])),
    [environments.data],
  );
  const myId = useAuth((s) => s.user?.id);
  const requests = (pending.data?.data.requests ?? []).filter((r) => r.requesterId !== myId);

  return (
    <section aria-labelledby="oncall-approvals-title">
      <h1 id="oncall-approvals-title" className="mb-3 text-lg font-semibold">
        {t('approvals.title')}
      </h1>
      <StaleNotice cachedAt={pending.data?.cachedAt ?? null} />
      <QueryState
        isLoading={pending.isLoading}
        error={pending.error}
        onRetry={() => void pending.refetch()}
        missingPermission={canDecide ? null : PERM.USERS_MANAGE}
        isEmpty={requests.length === 0}
        emptyText={t('approvals.empty')}
      >
        <ul className="flex flex-col gap-2">
          {requests.map((request) => (
            <ApprovalCard
              key={request.id}
              request={request}
              requester={userNames.get(request.requesterId) ?? request.requesterId}
              environment={
                request.environmentId ? (envNames.get(request.environmentId) ?? request.environmentId) : undefined
              }
            />
          ))}
        </ul>
      </QueryState>
    </section>
  );
}

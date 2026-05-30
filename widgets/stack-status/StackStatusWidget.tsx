// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { api, type PaginatedData } from '@core/api/client';
import { useEnvironmentStore } from '@resources/stores/environment';
import { StatusBadge, STACK_STATUS } from '@resources/components/ui/StatusBadge';
import type { WidgetTypeId } from '@modules/dashboard/widgets/registry';

type StackSummary = {
  id: string;
  name: string;
  status: string;
  serviceCount: number;
};

function useStacks() {
  const envId = useEnvironmentStore((s) => s.currentId);
  return useQuery({
    queryKey: ['stacks-widget', envId],
    queryFn: () =>
      api
        .get<PaginatedData<StackSummary>>('/stacks', envId ? { env: envId } : {})
        .then((r) => r.data?.items ?? []),
    refetchInterval: 15_000,
  });
}

export default function StackStatusWidget({ typeId: _typeId }: { colSpan: number; typeId: WidgetTypeId }) {
  const { t } = useTranslation('dashboard');
  const { data: stacks, isLoading } = useStacks();

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('stackStatusWidget.loadingStacks')}
      </div>
    );
  }

  if (!stacks || stacks.length === 0) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t('stackStatusWidget.noStacksFound')}
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h3 className="shrink-0 px-4 pt-3 pb-2 text-sm font-semibold text-foreground">{t('stackStatusWidget.title')}</h3>
      <div className="flex-1 overflow-y-auto px-4 pb-2">
        <div className="grid gap-2">
          {stacks.map((stack) => (
            <div key={stack.id} className="flex items-center justify-between rounded-md border border-border/50 px-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-xs font-medium text-foreground">{stack.name}</p>
                <p className="text-[10px] text-muted-foreground">
                  {t('stackStatusWidget.service', { count: stack.serviceCount })}
                </p>
              </div>
              <StatusBadge status={stack.status} map={STACK_STATUS} className="text-[10px] px-1.5 py-0.5" />
            </div>
          ))}
        </div>
      </div>
      <div className="shrink-0 border-t border-border px-4 py-2">
        <Link to="/stacks" className="text-xs text-primary hover:underline">
          {t('stackStatusWidget.viewAll')} &rarr;
        </Link>
      </div>
    </div>
  );
}

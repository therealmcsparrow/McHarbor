// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { ListItem } from '@resources/components/ui/ListItem';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { timeAgo } from '@resources/utils/format';
import type { ScheduleItem } from '../types';
import { CreateScheduleDialog } from './CreateScheduleDialog';

export function SchedulesTab() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();

  const { data: schedules = [], isLoading } = useQuery({
    queryKey: ['schedules'],
    queryFn: () =>
      api
        .get<PaginatedData<ScheduleItem>>('/schedules', { per_page: '100' })
        .then((r) => r.data?.items ?? []),
  });

  const deleteSchedule = useMutation({
    mutationFn: (id: string) => api.del(`/schedules/${id}`).then(assertSuccess),
    meta: { success: t('toast.scheduleDeleted') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['schedules'] }),
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [confirmTarget, setConfirmTarget] = useState<string | null>(null);

  if (isLoading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {t('schedules.description')}
        </p>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <IconPlus className="h-4 w-4" /> {t('schedules.addSchedule')}
        </Button>
      </div>

      {schedules.length === 0 ? (
        <div className="py-8 text-center text-muted-foreground">
          {t('schedules.noSchedules')}
        </div>
      ) : (
        <div className="space-y-2">
          {schedules.map((sch) => (
            <ListItem
              key={sch.id}
              title={sch.name}
              subtitle={`${sch.cron}  \u00b7  ${sch.action} \u2192 ${sch.target}${sch.lastRunAt ? `  \u00b7  Last: ${timeAgo(sch.lastRunAt)}` : ''}`}
              badge={
                <Badge variant={sch.enabled ? 'success' : 'secondary'}>
                  {sch.enabled ? t('schedules.active') : t('schedules.inactive')}
                </Badge>
              }
              actions={
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('schedules.deleteSchedule')}
                  onClick={() => setConfirmTarget(sch.id)}
                >
                  <IconTrash className="h-4 w-4 text-destructive" />
                </Button>
              }
            />
          ))}
        </div>
      )}

      <CreateScheduleDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('schedules.confirmDeleteTitle')}
        description={t('schedules.confirmDeleteDescription')}
        confirmLabel={t('schedules.confirmDeleteLabel')}
        onConfirm={() => {
          if (confirmTarget) deleteSchedule.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={deleteSchedule.isPending}
      />
    </div>
  );
}

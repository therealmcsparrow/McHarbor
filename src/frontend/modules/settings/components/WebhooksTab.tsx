// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { IconPlus, IconTrash, IconSend } from '@tabler/icons-react';
import { api, type PaginatedData } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { ListItem } from '@resources/components/ui/ListItem';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import type { WebhookItem } from '../types';
import { CreateWebhookDialog } from './CreateWebhookDialog';

export function WebhooksTab() {
  const { t } = useTranslation('settings');
  const queryClient = useQueryClient();

  const { data: webhooks = [], isLoading } = useQuery({
    queryKey: ['webhooks'],
    queryFn: () =>
      api
        .get<PaginatedData<WebhookItem>>('/webhooks', { per_page: '100' })
        .then((r) => r.data?.items ?? []),
  });

  const deleteWebhook = useMutation({
    mutationFn: (id: string) => api.del(`/webhooks/${id}`).then(assertSuccess),
    meta: { success: t('toast.webhookDeleted') },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['webhooks'] }),
  });

  const testWebhook = useMutation({
    mutationFn: (id: string) => api.post(`/webhooks/${id}/test`).then(assertSuccess),
    meta: { success: t('toast.webhookTestSent') },
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
          {t('webhooks.description')}
        </p>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <IconPlus className="h-4 w-4" /> {t('webhooks.addWebhook')}
        </Button>
      </div>

      {webhooks.length === 0 ? (
        <div className="py-8 text-center text-muted-foreground">{t('webhooks.noWebhooks')}</div>
      ) : (
        <div className="space-y-2">
          {webhooks.map((wh) => (
            <ListItem
              key={wh.id}
              title={wh.name}
              subtitle={wh.url}
              badge={
                <Badge variant={wh.isActive ? 'success' : 'secondary'}>
                  {wh.isActive ? t('webhooks.active') : t('webhooks.inactive')}
                </Badge>
              }
              actions={
                <>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('webhooks.testWebhook')}
                    onClick={() => testWebhook.mutate(wh.id)}
                  >
                    <IconSend className="h-4 w-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t('webhooks.deleteWebhook')}
                    onClick={() => setConfirmTarget(wh.id)}
                  >
                    <IconTrash className="h-4 w-4 text-destructive" />
                  </Button>
                </>
              }
            />
          ))}
        </div>
      )}

      <CreateWebhookDialog open={createOpen} onOpenChange={setCreateOpen} />

      <ConfirmDialog
        open={confirmTarget !== null}
        onOpenChange={(open) => !open && setConfirmTarget(null)}
        title={t('webhooks.confirmDeleteTitle')}
        description={t('webhooks.confirmDeleteDescription')}
        confirmLabel={t('webhooks.confirmDeleteLabel')}
        onConfirm={() => {
          if (confirmTarget) deleteWebhook.mutate(confirmTarget);
          setConfirmTarget(null);
        }}
        loading={deleteWebhook.isPending}
      />
    </div>
  );
}

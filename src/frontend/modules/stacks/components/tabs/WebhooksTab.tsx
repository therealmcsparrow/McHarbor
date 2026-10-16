// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useStackWebhooks,
  useCreateStackWebhook,
  useUpdateStackWebhook,
  useDeleteStackWebhook,
  useTestStackWebhook,
} from '@resources/hooks/useStacks';
import { WEBHOOK_EVENTS } from '@resources/types/stack-webhook';
import type { WebhookEvent } from '@resources/types/stack-webhook';
import { WebhooksTable } from './WebhooksTable';
import { PERM } from '@core/auth/permissionKeys';

type WebhooksTabProps = {
  stackName: string;
};

export function WebhooksTab({ stackName }: WebhooksTabProps) {
  const { t } = useTranslation('stacks');
  const { data: webhooks, isLoading } = useStackWebhooks(stackName);
  const createWebhook = useCreateStackWebhook();
  const updateWebhook = useUpdateStackWebhook();
  const deleteWebhook = useDeleteStackWebhook();
  const testWebhook = useTestStackWebhook();

  const [showForm, setShowForm] = useState(false);
  const [formUrl, setFormUrl] = useState('');
  const [formSecret, setFormSecret] = useState('');
  const [formEvents, setFormEvents] = useState<WebhookEvent[]>(['up', 'down', 'restart']);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);

  const resetForm = () => {
    setFormUrl('');
    setFormSecret('');
    setFormEvents(['up', 'down', 'restart']);
    setShowForm(false);
  };

  const handleCreate = () => {
    createWebhook.mutate(
      {
        name: stackName,
        input: {
          url: formUrl,
          secret: formSecret || undefined,
          events: JSON.stringify(formEvents),
        },
      },
      { onSuccess: resetForm },
    );
  };

  const handleToggleActive = (webhookId: string, isActive: boolean) => {
    updateWebhook.mutate({
      stackName,
      webhookId,
      input: { isActive: !isActive },
    });
  };

  const handleTest = (webhookId: string) => {
    setTestingId(webhookId);
    testWebhook.mutate({ stackName, webhookId }, { onSettled: () => setTestingId(null) });
  };

  const handleDelete = () => {
    if (!deleteConfirmId) return;
    deleteWebhook.mutate({ stackName, webhookId: deleteConfirmId }, { onSuccess: () => setDeleteConfirmId(null) });
  };

  const toggleEvent = (webhookEvent: WebhookEvent) => {
    setFormEvents((prev) =>
      prev.includes(webhookEvent) ? prev.filter((e) => e !== webhookEvent) : [...prev, webhookEvent],
    );
  };

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium">{t('webhooks.title')}</h3>
        <Button variant="outline" size="sm" onClick={() => setShowForm(true)} disabled={showForm}>
          <IconPlus className="mr-1 size-3.5" />
          {t('webhooks.addWebhook')}
        </Button>
      </div>

      {/* Create form */}
      {showForm && (
        <div className="mc-glass space-y-3 rounded-lg p-4">
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.url')}</label>
            <input
              type="url"
              value={formUrl}
              onChange={(e) => setFormUrl(e.target.value)}
              placeholder={t('webhooks.urlPlaceholder')}
              className="mc-control w-full rounded-md border border-border bg-muted px-3 text-sm text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.secret')}</label>
            <input
              type="text"
              value={formSecret}
              onChange={(e) => setFormSecret(e.target.value)}
              placeholder={t('webhooks.secretPlaceholder')}
              className="mc-control w-full rounded-md border border-border bg-muted px-3 text-sm text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.events')}</label>
            <div className="flex flex-wrap gap-2">
              {WEBHOOK_EVENTS.map((webhookEvent) => (
                <Button
                  key={webhookEvent}
                  type="button"
                  variant="ghost"
                  onClick={() => toggleEvent(webhookEvent)}
                  className={`rounded-md border px-2.5 py-1 text-xs font-medium transition-colors ${
                    formEvents.includes(webhookEvent)
                      ? 'border-primary bg-primary/10 text-primary'
                      : 'border-border bg-muted text-muted-foreground hover:border-primary/50'
                  }`}
                >
                  {t(`webhooks.event${webhookEvent.charAt(0).toUpperCase() + webhookEvent.slice(1)}`)}
                </Button>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Button
              permission={PERM.STACKS_MANAGE}
              size="sm"
              onClick={handleCreate}
              disabled={!formUrl || createWebhook.isPending}
            >
              {t('webhooks.addWebhook')}
            </Button>
            <Button variant="outline" size="sm" onClick={resetForm}>
              {t('actions.cancel')}
            </Button>
          </div>
        </div>
      )}

      {/* Webhooks table */}
      {webhooks && webhooks.length > 0 ? (
        <WebhooksTable
          webhooks={webhooks}
          testingId={testingId}
          onToggleActive={handleToggleActive}
          onTest={handleTest}
          onDelete={setDeleteConfirmId}
        />
      ) : (
        !showForm && <p className="py-12 text-center text-sm text-muted-foreground">{t('webhooks.noWebhooks')}</p>
      )}

      <ConfirmDialog
        open={!!deleteConfirmId}
        onOpenChange={(next) => !next && setDeleteConfirmId(null)}
        title={t('webhooks.delete')}
        description={t('webhooks.deleteConfirm')}
        confirmLabel={t('webhooks.delete')}
        onConfirm={handleDelete}
        loading={deleteWebhook.isPending}
      />
    </div>
  );
}

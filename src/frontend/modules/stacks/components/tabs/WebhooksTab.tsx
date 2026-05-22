// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconPlus,
  IconTrash,
  IconTestPipe,
  IconLoader2,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import {
  useStackWebhooks,
  useCreateStackWebhook,
  useUpdateStackWebhook,
  useDeleteStackWebhook,
  useTestStackWebhook,
} from '../../hooks/useStacks';
import { WEBHOOK_EVENTS } from '../../types/stack-webhook';
import type { WebhookEvent } from '../../types/stack-webhook';

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
    testWebhook.mutate(
      { stackName, webhookId },
      { onSettled: () => setTestingId(null) },
    );
  };

  const handleDelete = () => {
    if (!deleteConfirmId) return;
    deleteWebhook.mutate(
      { stackName, webhookId: deleteConfirmId },
      { onSuccess: () => setDeleteConfirmId(null) },
    );
  };

  const toggleEvent = (event: WebhookEvent) => {
    setFormEvents((prev) =>
      prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event],
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
        <div className="rounded-lg border border-border bg-card p-4 space-y-3">
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.url')}</label>
            <input
              type="url"
              value={formUrl}
              onChange={(e) => setFormUrl(e.target.value)}
              placeholder={t('webhooks.urlPlaceholder')}
              className="w-full rounded-md border border-border bg-muted px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.secret')}</label>
            <input
              type="text"
              value={formSecret}
              onChange={(e) => setFormSecret(e.target.value)}
              placeholder={t('webhooks.secretPlaceholder')}
              className="w-full rounded-md border border-border bg-muted px-3 py-2 text-sm text-foreground focus:border-primary focus:outline-none"
            />
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">{t('webhooks.events')}</label>
            <div className="flex flex-wrap gap-2">
              {WEBHOOK_EVENTS.map((event) => (
                <button
                  key={event}
                  type="button"
                  onClick={() => toggleEvent(event)}
                  className={`rounded-md border px-2.5 py-1 text-xs font-medium transition-colors ${
                    formEvents.includes(event)
                      ? 'border-primary bg-primary/10 text-primary'
                      : 'border-border bg-muted text-muted-foreground hover:border-primary/50'
                  }`}
                >
                  {t(`webhooks.event${event.charAt(0).toUpperCase() + event.slice(1)}`)}
                </button>
              ))}
            </div>
          </div>
          <div className="flex items-center gap-2 pt-1">
            <Button size="sm" onClick={handleCreate} disabled={!formUrl || createWebhook.isPending}>
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
        <div className="overflow-hidden rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
                  {t('webhooks.url')}
                </th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">
                  {t('webhooks.events')}
                </th>
                <th className="px-4 py-2.5 text-center font-medium text-muted-foreground">
                  {t('webhooks.active')}
                </th>
                <th className="w-24 px-4 py-2.5" />
              </tr>
            </thead>
            <tbody>
              {webhooks.map((wh) => {
                const events: string[] = JSON.parse(wh.events || '[]');
                return (
                  <tr key={wh.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                    <td className="px-4 py-2.5 font-mono text-xs truncate max-w-xs">
                      {wh.url}
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex flex-wrap gap-1">
                        {events.map((e) => (
                          <Badge key={e} variant="secondary" className="text-[10px] px-1.5 py-0">
                            {e}
                          </Badge>
                        ))}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-center">
                      <button
                        type="button"
                        onClick={() => handleToggleActive(wh.id, wh.isActive)}
                        className={`inline-flex size-5 items-center justify-center rounded-full transition-colors ${
                          wh.isActive ? 'bg-green-500/20 text-green-500' : 'bg-muted text-muted-foreground'
                        }`}
                        aria-label={t('webhooks.active')}
                      >
                        <div className={`size-2.5 rounded-full ${wh.isActive ? 'bg-green-500' : 'bg-muted-foreground/50'}`} />
                      </button>
                    </td>
                    <td className="px-4 py-2.5">
                      <div className="flex items-center justify-end gap-1">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => handleTest(wh.id)}
                              disabled={testingId === wh.id}
                              aria-label={t('webhooks.test')}
                            >
                              {testingId === wh.id ? (
                                <IconLoader2 className="size-3.5 animate-spin text-muted-foreground" />
                              ) : (
                                <IconTestPipe className="size-3.5 text-muted-foreground" />
                              )}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t('webhooks.test')}</TooltipContent>
                        </Tooltip>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => setDeleteConfirmId(wh.id)}
                              aria-label={t('webhooks.delete')}
                              className="hover:text-red-500"
                            >
                              <IconTrash className="size-3.5 text-muted-foreground" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t('webhooks.delete')}</TooltipContent>
                        </Tooltip>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : (
        !showForm && (
          <p className="py-12 text-center text-sm text-muted-foreground">
            {t('webhooks.noWebhooks')}
          </p>
        )
      )}

      <ConfirmDialog
        open={!!deleteConfirmId}
        onOpenChange={(open) => !open && setDeleteConfirmId(null)}
        title={t('webhooks.delete')}
        description={t('webhooks.deleteConfirm')}
        confirmLabel={t('webhooks.delete')}
        onConfirm={handleDelete}
        loading={deleteWebhook.isPending}
      />
    </div>
  );
}

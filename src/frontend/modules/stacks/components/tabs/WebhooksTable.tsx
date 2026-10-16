// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash, IconTestPipe, IconLoader2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import type { StackWebhook } from '@resources/types/stack-webhook';
import { PERM } from '@core/auth/permissionKeys';

type WebhooksTableProps = {
  webhooks: StackWebhook[];
  testingId: string | null;
  onToggleActive: (webhookId: string, isActive: boolean) => void;
  onTest: (webhookId: string) => void;
  onDelete: (webhookId: string) => void;
};

export function WebhooksTable({ webhooks, testingId, onToggleActive, onTest, onDelete }: WebhooksTableProps) {
  const { t } = useTranslation('stacks');

  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted">
          <tr className="border-b border-border bg-muted/50">
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('webhooks.url')}</th>
            <th className="px-4 py-2.5 text-left font-medium text-muted-foreground">{t('webhooks.events')}</th>
            <th className="px-4 py-2.5 text-center font-medium text-muted-foreground">{t('webhooks.active')}</th>
            <th className="w-24 px-4 py-2.5">
              <span className="sr-only">{t('columns.actions')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {webhooks.map((wh) => {
            const events: string[] = JSON.parse(wh.events || '[]');
            return (
              <tr key={wh.id} className="border-b border-border last:border-0 hover:bg-muted/30">
                <td className="max-w-xs truncate px-4 py-2.5 font-mono text-xs">{wh.url}</td>
                <td className="px-4 py-2.5">
                  <div className="flex flex-wrap gap-1">
                    {events.map((e) => (
                      <Badge key={e} variant="secondary" className="px-1.5 py-0 text-[10px]">
                        {e}
                      </Badge>
                    ))}
                  </div>
                </td>
                <td className="px-4 py-2.5 text-center">
                  <Switch
                    checked={wh.isActive}
                    aria-label={t('webhooks.active')}
                    onCheckedChange={() => onToggleActive(wh.id, wh.isActive)}
                  />
                </td>
                <td className="px-4 py-2.5">
                  <span className="sr-only">{t('columns.actions')}</span>
                  <div className="flex items-center justify-end gap-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => onTest(wh.id)}
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
                          permission={PERM.STACKS_MANAGE}
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => onDelete(wh.id)}
                          aria-label={t('webhooks.delete')}
                          className="hover:text-danger"
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
  );
}

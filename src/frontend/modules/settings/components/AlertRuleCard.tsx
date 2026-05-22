// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPencil, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Switch } from '@resources/components/ui/Switch';
import { cn } from '@resources/utils/cn';
import { useDeleteAlert, useUpdateAlert, type AlertRule } from '../hooks/useAlerts';
import {
  ALERT_SEVERITY_BADGE_VARIANTS,
  ALERT_SEVERITY_LABEL_KEYS,
  ALERT_TYPE_LABEL_KEYS,
} from './alert-options';

type AlertRuleCardProps = {
  rule: AlertRule;
  channelName?: string;
  onEdit: (rule: AlertRule) => void;
};

export function AlertRuleCard({ rule, channelName, onEdit }: AlertRuleCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const updateAlert = useUpdateAlert();
  const deleteAlert = useDeleteAlert();
  const destinations = [
    channelName,
    rule.sendInApp ? t('alerts.inAppLabel') : undefined,
  ].filter((value): value is string => Boolean(value));

  function handleToggle(enabled: boolean) {
    updateAlert.mutate({ id: rule.id, enabled });
  }

  function handleDelete() {
    deleteAlert.mutate(rule.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="rounded-lg border border-border bg-card p-4">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="min-w-0 flex-1 space-y-4">
            <div className="flex flex-wrap items-center gap-2">
              <h4 className="truncate text-sm font-medium text-foreground">{rule.name}</h4>
              <Badge variant={ALERT_SEVERITY_BADGE_VARIANTS[rule.severity]}>
                {t(ALERT_SEVERITY_LABEL_KEYS[rule.severity])}
              </Badge>
              <Badge variant={rule.enabled ? 'success' : 'secondary'}>
                {rule.enabled ? t('alerts.enabled') : t('alerts.disabled')}
              </Badge>
            </div>

            <div className="grid gap-3 text-xs sm:grid-cols-2 xl:grid-cols-4">
              <div>
                <p className="text-muted-foreground">{t('alerts.typeLabel')}</p>
                <p className="mt-1 text-sm text-foreground">{t(ALERT_TYPE_LABEL_KEYS[rule.type])}</p>
              </div>
              <div>
                <p className="text-muted-foreground">{t('alerts.targetLabel')}</p>
                <p className="mt-1 truncate text-sm text-foreground">
                  {rule.target || t('alerts.allTargets')}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">{t('alerts.destinationsLabel')}</p>
                <p className="mt-1 truncate text-sm text-foreground">
                  {destinations.length > 0 ? destinations.join(', ') : t('alerts.unassignedChannel')}
                </p>
              </div>
              <div>
                <p className="text-muted-foreground">{t('alerts.statusLabel')}</p>
                <p
                  className={cn(
                    'mt-1 text-sm',
                    rule.enabled ? 'text-foreground' : 'text-muted-foreground'
                  )}
                >
                  {rule.enabled ? t('alerts.enabledDescription') : t('alerts.disabledDescription')}
                </p>
              </div>
            </div>

            <div className="rounded-lg border border-border bg-muted/30 px-3 py-2">
              <p className="text-xs text-muted-foreground">{t('alerts.conditionLabel')}</p>
              <p className="mt-1 font-mono text-xs text-foreground">
                {rule.condition || t('alerts.anyCondition')}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 lg:w-auto lg:flex-col lg:items-end">
            <div className="flex items-center gap-2 rounded-lg border border-border px-3 py-2">
              <span className="text-xs text-muted-foreground">
                {rule.enabled ? t('alerts.disableRule') : t('alerts.enableRule')}
              </span>
              <Switch
                aria-label={rule.enabled ? t('alerts.disableRule') : t('alerts.enableRule')}
                checked={rule.enabled}
                onCheckedChange={handleToggle}
                disabled={updateAlert.isPending}
              />
            </div>

            <div className="flex items-center gap-1">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('alerts.editRule')}
                onClick={() => onEdit(rule)}
              >
                <IconPencil className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t('alerts.deleteRule')}
                onClick={() => setDeleteOpen(true)}
              >
                <IconTrash className="size-4 text-destructive" />
              </Button>
            </div>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('alerts.confirmDeleteTitle')}
        description={t('alerts.confirmDeleteDescription', { name: rule.name })}
        confirmLabel={t('common:actions.delete')}
        loading={deleteAlert.isPending}
        onConfirm={handleDelete}
      />
    </>
  );
}

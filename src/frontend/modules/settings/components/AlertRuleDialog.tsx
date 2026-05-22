// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { Textarea } from '@resources/components/ui/Textarea';
import { useCreateAlert, useUpdateAlert, type AlertRule } from '../hooks/useAlerts';
import { type CommunicationChannel } from '../hooks/useNotificationChannels';
import {
  ALERT_SEVERITIES,
  ALERT_SEVERITY_LABEL_KEYS,
  ALERT_TYPES,
  ALERT_TYPE_LABEL_KEYS,
} from './alert-options';

type AlertRuleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channels: CommunicationChannel[];
  alert?: AlertRule | null;
};

type AlertFormState = {
  name: string;
  severity: AlertRule['severity'];
  type: AlertRule['type'];
  condition: string;
  target: string;
  channelId: string;
  sendInApp: boolean;
};

const DEFAULT_FORM: AlertFormState = {
  name: '',
  severity: 'warning',
  type: 'cpu',
  condition: '',
  target: '*',
  channelId: '',
  sendInApp: false,
};

export function AlertRuleDialog({ open, onOpenChange, channels, alert }: AlertRuleDialogProps) {
  const { t } = useTranslation('settings');
  const { t: tc } = useTranslation('common');
  const createAlert = useCreateAlert();
  const updateAlert = useUpdateAlert();
  const isEditing = Boolean(alert);
  const [form, setForm] = useState<AlertFormState>(DEFAULT_FORM);

  useEffect(() => {
    if (!open) return;

    if (alert) {
      setForm({
        name: alert.name,
        severity: alert.severity,
        type: alert.type,
        condition: alert.condition,
        target: alert.target || '*',
        channelId: alert.channelId || '',
        sendInApp: alert.sendInApp,
      });
      return;
    }

    setForm(DEFAULT_FORM);
  }, [alert, channels, open]);

  const channelOptions = [
    {
      value: '',
      label: t('alerts.noChannelOption'),
    },
    ...channels.map((channel) => ({
      value: channel.id,
      label: channel.enabled
        ? channel.name
        : `${channel.name} (${t('communications.disabled')})`,
    })),
  ];
  if (form.channelId && !channelOptions.some((channel) => channel.value === form.channelId)) {
    channelOptions.unshift({
      value: form.channelId,
      label: t('alerts.unassignedChannel'),
    });
  }
  const isPending = createAlert.isPending || updateAlert.isPending;
  const canSave = !!(form.name.trim() && (form.channelId || form.sendInApp));

  function handleClose(nextOpen: boolean) {
    if (!nextOpen) {
      setForm(DEFAULT_FORM);
    }
    onOpenChange(nextOpen);
  }

  function handleSave() {
    const payload = {
      name: form.name.trim(),
      severity: form.severity,
      type: form.type,
      condition: form.condition.trim(),
      target: form.target.trim(),
      channelId: form.channelId,
      sendInApp: form.sendInApp,
    };

    if (alert) {
      updateAlert.mutate(
        {
          id: alert.id,
          ...payload,
        },
        {
          onSuccess: () => handleClose(false),
        }
      );
      return;
    }

    createAlert.mutate(payload, {
      onSuccess: () => handleClose(false),
    });
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isEditing ? t('alerts.editTitle') : t('alerts.createTitle')}</DialogTitle>
          <DialogDescription>
            {isEditing ? t('alerts.editDescription') : t('alerts.createDescription')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-5">
          <div className="grid gap-5 sm:grid-cols-2">
            <div className="space-y-2 sm:col-span-2">
              <Label>{t('alerts.nameLabel')}</Label>
              <Input
                value={form.name}
                onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                placeholder={t('alerts.namePlaceholder')}
              />
            </div>

            <div className="space-y-2">
              <Label>{t('alerts.severityLabel')}</Label>
              <Select
                value={form.severity}
                onChange={(value) => setForm((current) => ({ ...current, severity: value as AlertRule['severity'] }))}
                options={ALERT_SEVERITIES.map((severity) => ({
                  value: severity,
                  label: t(ALERT_SEVERITY_LABEL_KEYS[severity]),
                }))}
                searchable={false}
              />
            </div>

            <div className="space-y-2">
              <Label>{t('alerts.typeLabel')}</Label>
              <Select
                value={form.type}
                onChange={(value) => setForm((current) => ({ ...current, type: value as AlertRule['type'] }))}
                options={ALERT_TYPES.map((type) => ({
                  value: type,
                  label: t(ALERT_TYPE_LABEL_KEYS[type]),
                }))}
                searchable={false}
              />
            </div>

            <div className="space-y-2">
              <Label>{t('alerts.targetLabel')}</Label>
              <Input
                value={form.target}
                onChange={(event) => setForm((current) => ({ ...current, target: event.target.value }))}
                placeholder={t('alerts.targetPlaceholder')}
              />
            </div>

            <div className="space-y-2">
              <Label>{t('alerts.channelLabel')}</Label>
              <Select
                value={form.channelId}
                onChange={(value) => setForm((current) => ({ ...current, channelId: value }))}
                options={channelOptions}
                placeholder={t('alerts.channelPlaceholder')}
              />
              <p className="text-xs text-muted-foreground">
                {channels.length === 0 ? t('alerts.noChannelsInlineHint') : t('alerts.channelRequiredHint')}
              </p>
            </div>

            <div className="space-y-3 rounded-lg border border-border bg-muted/20 p-4 sm:col-span-2">
              <div className="flex items-start justify-between gap-4">
                <div className="space-y-1">
                  <p className="text-sm font-medium text-foreground">{t('alerts.inAppLabel')}</p>
                  <p className="text-xs text-muted-foreground">{t('alerts.inAppDescription')}</p>
                </div>
                <Switch
                  aria-label={t('alerts.inAppLabel')}
                  checked={form.sendInApp}
                  onCheckedChange={(checked) => setForm((current) => ({ ...current, sendInApp: checked }))}
                />
              </div>
              {!canSave ? (
                <p className="text-xs text-muted-foreground">{t('alerts.destinationRequiredHint')}</p>
              ) : null}
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label>{t('alerts.conditionLabel')}</Label>
              <Textarea
                value={form.condition}
                onChange={(event) => setForm((current) => ({ ...current, condition: event.target.value }))}
                placeholder={t('alerts.conditionPlaceholder')}
                className="min-h-[120px]"
              />
            </div>
          </div>
        </DialogBody>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleClose(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={handleSave} disabled={isPending || !canSave}>
            {isPending
              ? (isEditing ? tc('actions.saving') : t('alerts.creating'))
              : (isEditing ? tc('actions.save') : tc('actions.create'))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

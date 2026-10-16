// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
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
import { useCreateAlert, useTestAlert, useUpdateAlert, type AlertRule } from '@resources/hooks/useAlerts';
import { findCondition, useAlertConditions } from '@resources/hooks/useAlertConditions';
import { DEFAULT_FORM, formFromRule, payloadFromForm, type AlertFormState } from './alert-rule-form';
import { type CommunicationChannel } from '../hooks/useNotificationChannels';
import { ALERT_SEVERITIES, ALERT_SEVERITY_LABEL_KEYS } from './alert-options';
import { AlertConditionFields } from './AlertConditionFields';
import { AlertDeliveryFields } from './AlertDeliveryFields';
import { AlertDestinationsField } from './AlertDestinationsField';
import { PERM } from '@core/auth/permissionKeys';

type AlertRuleDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channels: CommunicationChannel[];
  alert?: AlertRule | null;
  onOpenCommunications: () => void;
};

export function AlertRuleDialog({
  open: isOpen,
  onOpenChange,
  channels,
  alert: rule,
  onOpenCommunications,
}: AlertRuleDialogProps) {
  const { t } = useTranslation('settings');
  const { data: catalog } = useAlertConditions();
  const createAlert = useCreateAlert();
  const updateAlert = useUpdateAlert();
  const testAlert = useTestAlert();
  const isEditing = Boolean(rule);
  const [form, setForm] = useState<AlertFormState>(DEFAULT_FORM);

  const [seeded, setSeeded] = useState<{ rule: typeof rule; isOpen: boolean }>({ rule: undefined, isOpen: false });
  if (seeded.rule !== rule || seeded.isOpen !== isOpen) {
    setSeeded({ rule, isOpen });
    if (isOpen) setForm(rule ? formFromRule(rule) : DEFAULT_FORM);
  }

  const selected = findCondition(catalog, form.sourceType, form.conditionKey);
  const hasDestination = form.destinations.length > 0 || form.sendInApp;
  const canSave = Boolean(form.name.trim()) && Boolean(selected) && hasDestination;
  const isPending = createAlert.isPending || updateAlert.isPending;

  function handleClose(nextOpen: boolean) {
    if (!nextOpen) {
      setForm(DEFAULT_FORM);
    }
    onOpenChange(nextOpen);
  }

  /**
   * Switching condition resets the threshold to the new declaration's
   * defaults. Carrying "> 80%" across from CPU usage to a restart-loop
   * count would look configured while meaning something else entirely.
   */
  function handleSourceChange(sourceType: string, conditionKey: string) {
    const next = findCondition(catalog, sourceType, conditionKey);
    setForm((current) => ({
      ...current,
      sourceType,
      conditionKey,
      condition: {
        operator: next?.condition.defaultOperator,
        threshold: next?.condition.defaultThreshold,
        unit: next?.condition.unit,
        forSeconds: 0,
        windowSeconds: next?.condition.kind === 'count_in_window' ? 900 : 0,
        params: {},
      },
    }));
  }

  function handleSave() {
    const payload = payloadFromForm(form);

    if (rule) {
      updateAlert.mutate({ id: rule.id, ...payload }, { onSuccess: () => handleClose(false) });
      return;
    }

    createAlert.mutate(payload, { onSuccess: () => handleClose(false) });
  }

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{isEditing ? t('alerts.editTitle') : t('alerts.createTitle')}</DialogTitle>
          <DialogDescription>
            {isEditing ? t('alerts.editDescription') : t('alerts.createDescription')}
          </DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-6">
          <div className="grid gap-5 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>{t('alerts.nameLabel')}</Label>
              <Input
                value={form.name}
                onChange={(changeEvent) => setForm((current) => ({ ...current, name: changeEvent.target.value }))}
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
          </div>

          <AlertConditionFields
            catalog={catalog}
            sourceType={form.sourceType}
            conditionKey={form.conditionKey}
            condition={form.condition}
            target={form.target}
            onSourceChange={handleSourceChange}
            onConditionChange={(condition) => setForm((current) => ({ ...current, condition }))}
            onTargetChange={(target) => setForm((current) => ({ ...current, target }))}
          />

          <AlertDestinationsField
            channels={channels}
            destinations={form.destinations}
            sendInApp={form.sendInApp}
            onDestinationsChange={(destinations) => setForm((current) => ({ ...current, destinations }))}
            onSendInAppChange={(sendInApp) => setForm((current) => ({ ...current, sendInApp }))}
            onOpenCommunications={onOpenCommunications}
          />

          <AlertDeliveryFields
            environmentIds={form.environmentIds}
            cooldownSeconds={form.cooldownSeconds}
            notifyOnRecovery={form.notifyOnRecovery}
            messageTemplate={form.messageTemplate}
            alertName={form.name}
            severity={form.severity}
            sourceType={form.sourceType}
            conditionKey={form.conditionKey}
            condition={form.condition}
            showEnvironmentScope={selected ? selected.condition.targetKind !== 'none' : false}
            onEnvironmentIdsChange={(environmentIds) => setForm((current) => ({ ...current, environmentIds }))}
            onCooldownChange={(cooldownSeconds) => setForm((current) => ({ ...current, cooldownSeconds }))}
            onNotifyOnRecoveryChange={(notifyOnRecovery) => setForm((current) => ({ ...current, notifyOnRecovery }))}
            onMessageTemplateChange={(messageTemplate) => setForm((current) => ({ ...current, messageTemplate }))}
          />

          {!hasDestination ? (
            <p className="text-xs text-muted-foreground">{t('alerts.destinationRequiredHint')}</p>
          ) : null}
        </DialogBody>

        <DialogFooter>
          {rule ? (
            <Button
              permission={PERM.ALERTS_MANAGE}
              variant="outline"
              onClick={() => testAlert.mutate(rule.id)}
              disabled={testAlert.isPending}
            >
              {testAlert.isPending ? t('alerts.testing') : t('alerts.testRule')}
            </Button>
          ) : null}
          <Button variant="outline" onClick={() => handleClose(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.ALERTS_MANAGE} onClick={handleSave} disabled={isPending || !canSave}>
            {isPending
              ? isEditing
                ? t('actions.saving', { ns: 'common' })
                : t('alerts.creating')
              : isEditing
                ? t('actions.save', { ns: 'common' })
                : t('actions.create', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

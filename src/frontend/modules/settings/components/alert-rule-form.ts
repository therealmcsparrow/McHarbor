// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { AlertConditionConfig, AlertDestinationInput, AlertRule } from '@resources/hooks/useAlerts';

export type AlertFormState = {
  name: string;
  severity: AlertRule['severity'];
  sourceType: string;
  conditionKey: string;
  condition: AlertConditionConfig;
  target: string;
  environmentIds: string[];
  destinations: AlertDestinationInput[];
  sendInApp: boolean;
  cooldownSeconds: number;
  notifyOnRecovery: boolean;
  messageTemplate: string;
};

export const DEFAULT_FORM: AlertFormState = {
  name: '',
  severity: 'warning',
  sourceType: '',
  conditionKey: '',
  condition: {},
  target: '*',
  environmentIds: [],
  destinations: [],
  sendInApp: false,
  cooldownSeconds: 0,
  notifyOnRecovery: true,
  messageTemplate: '',
};

export function formFromRule(rule: AlertRule): AlertFormState {
  return {
    name: rule.name,
    severity: rule.severity,
    sourceType: rule.sourceType,
    conditionKey: rule.conditionKey,
    condition: rule.condition ?? {},
    target: rule.target || '*',
    environmentIds: rule.environmentIds ?? [],
    destinations: (rule.destinations ?? []).map((destination) => ({
      channelId: destination.channelId,
      minSeverity: destination.minSeverity,
    })),
    sendInApp: rule.sendInApp,
    cooldownSeconds: rule.cooldownSeconds,
    notifyOnRecovery: rule.notifyOnRecovery,
    messageTemplate: rule.messageTemplate,
  };
}

export function payloadFromForm(form: AlertFormState) {
  return {
    name: form.name.trim(),
    severity: form.severity,
    sourceType: form.sourceType,
    conditionKey: form.conditionKey,
    condition: form.condition,
    target: form.target.trim(),
    environmentIds: form.environmentIds,
    destinations: form.destinations,
    sendInApp: form.sendInApp,
    cooldownSeconds: form.cooldownSeconds,
    notifyOnRecovery: form.notifyOnRecovery,
    messageTemplate: form.messageTemplate,
  };
}

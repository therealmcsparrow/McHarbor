// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

export type StackWebhook = {
  id: string;
  stackId: string;
  url: string;
  secret?: string;
  events: string;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
};

export type CreateStackWebhookInput = {
  url: string;
  secret?: string;
  events: string;
};

export type UpdateStackWebhookInput = {
  url?: string;
  secret?: string;
  events?: string;
  isActive?: boolean;
};

export type PruneResult = {
  removed: string[];
  count: number;
};

export const WEBHOOK_EVENTS = ['up', 'down', 'stop', 'restart', 'deploy', 'update'] as const;
export type WebhookEvent = (typeof WEBHOOK_EVENTS)[number];

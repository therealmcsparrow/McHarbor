// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { http, HttpResponse } from 'msw';
import type { Secret } from '@core/types/secrets';
import { DeleteSecretDialog } from './DeleteSecretDialog';
import { describeDestructiveDialog } from '../../../../test/destructive';

const secret: Secret = {
  id: 's1',
  name: 'db-password',
  kind: 'password',
  scope: 'global',
  version: 1,
  generated: false,
  system: false,
  createdAt: '2026-09-25T00:00:00Z',
  updatedAt: '2026-09-25T00:00:00Z',
  referenceCount: 0,
  autoApply: false,
  staleCount: 0,
};

describeDestructiveDialog({
  name: 'DeleteSecretDialog',
  method: 'delete',
  path: '*/api/secrets/s1',
  permission: 'secrets.delete',
  confirmLabel: 'Delete',
  reportsSuccess: false,
  handlers: [http.get('*/api/secrets/s1/references', () => HttpResponse.json({ success: true, data: [] }))],
  render: ({ onOpenChange }) => <DeleteSecretDialog open secret={secret} onOpenChange={onOpenChange} />,
});

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { RotateAPIKeyDialog } from './RotateAPIKeyDialog';
import type { APIKey } from '../../hooks/useAPIKeys';
import { describeDestructiveDialog } from '../../../../test/destructive';

const key: APIKey = {
  id: 'k1',
  userId: 'u1',
  username: 'carlo',
  name: 'ci',
  keyPrefix: 'mch_abc...',
  scopes: [],
  roles: [],
  environmentIds: [],
  environmentNames: [],
  expiresAt: null,
  lastUsedAt: null,
  isActive: true,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  status: 'active',
  deletedAt: null,
};

describeDestructiveDialog({
  name: 'RotateAPIKeyDialog',
  method: 'post',
  path: '*/api/api-keys/k1/rotate',
  permission: 'api_keys.manage',
  confirmLabel: 'Rotate key',
  render: ({ onOpenChange, onSuccess }) => (
    <RotateAPIKeyDialog apiKey={key} onOpenChange={onOpenChange} onRotated={onSuccess} />
  ),
});

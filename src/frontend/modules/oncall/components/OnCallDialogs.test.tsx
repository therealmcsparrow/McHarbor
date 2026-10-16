// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describeDestructiveDialog } from '../../../test/destructive';
import type { ElevationRequest } from '@resources/hooks/useElevations';
import { DecisionDialog } from './DecisionDialog';
import { RestartDialog } from './RestartDialog';

const request: ElevationRequest = {
  id: 'el1',
  requesterId: 'u1',
  permissionKey: 'containers.manage',
  justification: 'web is down',
  requestedMinutes: 30,
  status: 'pending',
  createdAt: '2026-09-26T03:00:00Z',
};

describeDestructiveDialog({
  name: 'On-call RestartDialog',
  method: 'post',
  path: '*/api/containers/c1/restart',
  permission: 'containers.manage',
  confirmLabel: 'Restart',
  render: ({ onOpenChange, onSuccess }) => (
    <RestartDialog envId="e1" containerId="c1" name="web" open onOpenChange={onOpenChange} onSuccess={onSuccess} />
  ),
});

for (const approve of [false, true]) {
  describeDestructiveDialog({
    name: `On-call DecisionDialog (${approve ? 'approve' : 'deny'})`,
    method: 'post',
    path: '*/api/elevations/el1/decide',
    permission: 'users.manage',
    confirmLabel: approve ? 'Approve' : 'Deny',
    body: { approve, note: '' },
    reportsSuccess: false,
    render: ({ onOpenChange }) => (
      <DecisionDialog request={request} requester="alice" approve={approve} open onOpenChange={onOpenChange} />
    ),
  });
}

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { ApprovalDecisionDialog } from './ApprovalDecisionDialog';
import type { ApprovalRequest } from '../../hooks/useApprovals';
import { describeDestructiveDialog } from '../../../../test/destructive';

const request: ApprovalRequest = {
  id: 'ap1',
  ruleId: 'r1',
  category: 'container.remove',
  requesterId: 'u2',
  requesterName: 'bob',
  environmentId: 'env-1',
  method: 'DELETE',
  path: '/api/containers/web?env=env-1',
  routePattern: '/api/containers/{id}',
  resourceName: 'web',
  bodyRedacted: '',
  status: 'pending',
  requiredApprovers: 1,
  approvalsCount: 0,
  approverPermission: 'approvals.decide',
  expiresAt: '2026-09-27T00:00:00Z',
  decidedAt: null,
  executedAt: null,
  resultStatus: null,
  resultCode: '',
  createdAt: '2026-09-26T00:00:00Z',
  decisions: [],
};

describeDestructiveDialog({
  name: 'ApprovalDecisionDialog (approve)',
  method: 'post',
  path: '*/api/approvals/ap1/decide',
  permission: 'approvals.decide',
  confirmLabel: 'Approve',
  body: { approve: true, note: '' },
  render: ({ onOpenChange }) => <ApprovalDecisionDialog request={request} approve onOpenChange={onOpenChange} />,
  reportsSuccess: false,
});

describeDestructiveDialog({
  name: 'ApprovalDecisionDialog (deny)',
  method: 'post',
  path: '*/api/approvals/ap1/decide',
  permission: 'approvals.decide',
  confirmLabel: 'Deny',
  body: { approve: false, note: '' },
  render: ({ onOpenChange }) => (
    <ApprovalDecisionDialog request={request} approve={false} onOpenChange={onOpenChange} />
  ),
  reportsSuccess: false,
});

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DeleteRoleDialog } from '@/modules/security/components/DeleteRoleDialog';
import {
  ApprovalRequiredError,
  assertSuccess,
  describeApiError,
  isApprovalRequired,
} from '@resources/utils/api-mutation';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

const held = {
  success: false,
  error: 'Forbidden',
  code: 'err.forbidden',
  data: { reason: 'approval_required', approvalId: 'ap42', category: 'role.delete', requiredApprovers: 1 },
};

describe('approval-held responses', () => {
  it('assertSuccess throws ApprovalRequiredError carrying the approval id', () => {
    let caught: unknown;
    try {
      assertSuccess(held);
    } catch (err) {
      caught = err;
    }
    expect(isApprovalRequired(caught)).toBe(true);
    expect((caught as ApprovalRequiredError).approvalId).toBe('ap42');
    expect((caught as Error).message).toContain('Sent for approval');
    expect((caught as Error).message).toContain('ap42');
  });

  it('describeApiError explains a hold instead of echoing Forbidden', () => {
    expect(describeApiError(held)).toContain('Sent for approval');
    expect(describeApiError({ error: 'Forbidden', data: { reason: 'other' } })).toBe('Forbidden');
  });

  describe('in a destructive confirm dialog', () => {
    const calls = vi.fn();
    beforeEach(() => {
      calls.mockReset();
      grantPermissions(['roles.manage']);
      server.use(
        http.delete('*/api/roles/r1', () => {
          calls();
          return HttpResponse.json(held, { status: 202 });
        }),
      );
    });

    it('shows sent-for-approval, reports no success, and does not resend', async () => {
      const user = userEvent.setup();
      const onOpenChange = vi.fn();
      const onSuccess = vi.fn();
      renderWithProviders(<DeleteRoleDialog roleId="r1" onOpenChange={onOpenChange} onSuccess={onSuccess} />);
      const dialog = await screen.findByRole('dialog');
      const confirm = within(dialog).getByRole('button', { name: 'Remove' });
      await user.click(confirm);
      const status = await within(dialog).findByRole('status');
      expect(status).toHaveTextContent('Sent for approval');
      expect(status).toHaveTextContent('ap42');
      expect(within(dialog).queryByRole('alert')).toBeNull();
      expect(onSuccess).not.toHaveBeenCalled();
      expect(onOpenChange).not.toHaveBeenCalledWith(false);
      await waitFor(() => expect(confirm).toBeDisabled());
      await user.click(confirm);
      expect(calls).toHaveBeenCalledTimes(1);
    });
  });
});

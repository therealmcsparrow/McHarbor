// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../../test/permissions';
import { renderWithProviders } from '../../../../test/render';
import { server } from '../../../../test/server';
import { MappingPreviewPanel } from './MappingPreviewPanel';

const config = {
  groupMappingEnabled: true,
  groupMappings: [{ providerGroup: 'ops', mcharborGroupId: 'g-ops' }],
  autoImportGroups: false,
  autoProvision: true,
};

const preview = {
  sample: {
    groups: [
      { groupId: 'g-ops', groupName: 'Operations', from: ['ops'], roles: [{ roleId: 'r1', roleName: 'Operator' }] },
    ],
    unmapped: ['sales'],
    roles: [{ roleId: 'r1', roleName: 'Operator' }],
  },
  impact: {
    evaluated: 2,
    unchanged: 1,
    users: [
      {
        userId: 'u1',
        username: 'ann',
        gained: [],
        lost: [{ roleId: 'r2', roleName: 'Viewer' }],
        joined: [],
        left: [{ groupId: 'g-dev', groupName: 'Developers' }],
      },
    ],
    unknown: [{ userId: 'u3', username: 'cid' }],
  },
};

describe('MappingPreviewPanel', () => {
  it('sends the typed sample and shows groups, roles and who gains or loses', async () => {
    grantPermissions(['identity_providers.manage']);
    const bodies: unknown[] = [];
    server.use(
      http.post('*/api/identity-providers/mapping-preview', async ({ request }) => {
        bodies.push(await request.json());
        return HttpResponse.json({ success: true, data: preview });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<MappingPreviewPanel providerId="p1" providerType="saml_2_0" config={config} />);
    await user.type(screen.getByLabelText('Provider groups, one per line'), 'ops, sales');
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    const joined = await screen.findByRole('list', { name: 'Groups joined' });
    expect(within(joined).getByText('Operations')).toBeInTheDocument();
    expect(within(joined).getByText('Operator · global')).toBeInTheDocument();
    expect(screen.getByText('Not mapped: sales')).toBeInTheDocument();
    const affected = screen.getByRole('list', { name: 'Affected users' });
    expect(within(affected).getByText('Viewer · global')).toBeInTheDocument();
    expect(within(affected).getByText('Leaves: Developers')).toBeInTheDocument();
    expect(screen.getByText(/no sign-in recorded since group claims were tracked: cid/)).toBeInTheDocument();
    expect(bodies).toEqual([{ providerId: 'p1', providerType: 'saml_2_0', config, sampleGroups: ['ops', 'sales'] }]);
  });
});

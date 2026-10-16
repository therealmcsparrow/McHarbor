// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../../test/render';
import { EffectivePermissionTable } from './EffectivePermissionTable';
import type { EffectivePermissions } from '../../hooks/useAccessReview';

const data: EffectivePermissions = {
  userId: 'u1',
  username: 'ann',
  environmentId: 'env-1',
  wildcard: false,
  breakGlass: false,
  abacEnforced: false,
  stackScopes: [],
  environmentNames: { 'env-1': 'Production' },
  permissions: [
    {
      key: 'containers.manage',
      effective: true,
      authorizeGrant: true,
      sources: [
        {
          kind: 'role',
          roleId: 'r1',
          roleName: 'Operator',
          via: 'group',
          groupId: 'g1',
          groupName: 'Ops',
          environmentId: 'env-1',
        },
      ],
    },
    {
      key: 'images.view',
      effective: true,
      authorizeGrant: false,
      sources: [
        { kind: 'role', roleId: 'r2', roleName: 'Viewer', via: 'user' },
        { kind: 'override_deny', overrideId: 'o1' },
      ],
    },
    {
      key: 'volumes.delete',
      effective: true,
      authorizeGrant: true,
      sources: [{ kind: 'elevation', elevationId: 'e1', until: '2026-09-27T10:00:00Z' }],
    },
    { key: 'networks.manage', effective: false, authorizeGrant: false, sources: [] },
  ],
};

describe('EffectivePermissionTable', () => {
  it('names where each permission comes from', () => {
    renderWithProviders(<EffectivePermissionTable data={data} filter="" showAll={false} />);
    expect(screen.getByText('Role Operator via group Ops, in Production')).toBeInTheDocument();
    expect(screen.getByText('Role Viewer, global')).toBeInTheDocument();
    expect(screen.getByText('Deny override, global')).toBeInTheDocument();
    expect(screen.getByText('Elevation until 2026-09-27T10:00:00Z')).toBeInTheDocument();
    expect(screen.queryByText('networks.manage')).not.toBeInTheDocument();
  });

  it('shows ungranted keys on request and filters by key', () => {
    renderWithProviders(<EffectivePermissionTable data={data} filter="network" showAll />);
    expect(screen.getByText('networks.manage')).toBeInTheDocument();
    expect(screen.queryByText('containers.manage')).not.toBeInTheDocument();
  });
});

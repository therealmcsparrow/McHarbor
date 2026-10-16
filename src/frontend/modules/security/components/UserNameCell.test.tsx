// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { UserNameCell } from './UserNameCell';
import type { UserItem } from '@resources/hooks/useUsers';

const base: UserItem = {
  id: 'u1',
  username: 'alice',
  displayName: null,
  email: null,
  role: 'user',
  isActive: true,
  lastLogin: null,
  lockedUntil: null,
  createdAt: '2026-09-27T00:00:00Z',
  updatedAt: '2026-09-27T00:00:00Z',
};

describe('UserNameCell', () => {
  it('labels a service principal as one', () => {
    renderWithProviders(<UserNameCell user={{ ...base, username: 'runbook-bot', kind: 'service' }} />);
    expect(screen.getByText('runbook-bot')).toBeInTheDocument();
    expect(screen.getByText('Service principal')).toBeInTheDocument();
  });

  it('does not label a person', () => {
    renderWithProviders(<UserNameCell user={{ ...base, kind: 'user' }} />);
    expect(screen.getByText('alice')).toBeInTheDocument();
    expect(screen.queryByText('Service principal')).not.toBeInTheDocument();
  });
});

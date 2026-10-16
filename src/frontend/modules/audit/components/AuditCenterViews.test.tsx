// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';
import { AITrailActionCard } from './AITrailActionCard';
import { AuditDenialsTable } from './AuditDenialsTable';
import { auditExportParams } from '../hooks/useAuditCompliance';
import { renderWithProviders } from '../../../test/render';

describe('AuditDenialsTable', () => {
  it('links the actor to the user and labels global and policy denials', () => {
    renderWithProviders(
      <MemoryRouter>
        <AuditDenialsTable
          envName={() => 'prod'}
          rows={[
            {
              id: 'd1',
              timestamp: '2026-09-26T10:00:00Z',
              kind: 'rbac',
              userId: 'u1',
              username: 'alice',
              permission: 'stacks.manage',
              route: '/api/stacks',
              environmentId: 'e1',
            },
            {
              id: 'd2',
              timestamp: '2026-09-26T10:01:00Z',
              kind: 'abac',
              userId: '',
              username: '',
              permission: 'stacks.delete',
              route: '',
              policyId: 'pol1',
            },
          ]}
        />
      </MemoryRouter>,
    );
    expect(screen.getByRole('link', { name: 'Open user alice' })).toHaveAttribute('href', '/security/users/u1');
    expect(screen.getByText('Unknown user')).toBeInTheDocument();
    expect(screen.getByText('Policy pol1')).toBeInTheDocument();
    expect(screen.getByText('Global')).toBeInTheDocument();
    expect(screen.getByText('prod')).toBeInTheDocument();
  });
});

describe('AITrailActionCard', () => {
  it('shows each step in order and says when the action record is gone', () => {
    renderWithProviders(
      <ul>
        <AITrailActionCard
          group={{
            actionId: 'act1',
            tool: 'restart_container',
            username: 'alice',
            status: 'executed',
            startedAt: '2026-09-26T10:00:00Z',
            lastAt: '2026-09-26T10:02:00Z',
            truncated: false,
            recordMissing: true,
            steps: [
              { kind: 'preview', timestamp: '2026-09-26T10:00:00Z' },
              { kind: 'approved', auditId: 'a1', timestamp: '2026-09-26T10:01:00Z' },
              { kind: 'executed', auditId: 'a2', timestamp: '2026-09-26T10:02:00Z' },
            ],
          }}
        />
      </ul>,
    );
    const steps = screen
      .getAllByRole('listitem')
      .slice(1)
      .map((item) => item.textContent ?? '');
    expect(steps[0]).toMatch(/^Preview/);
    expect(steps[1]).toMatch(/^Approved/);
    expect(steps[2]).toMatch(/^Executed/);
    expect(screen.getByText(/only the audit events remain/)).toBeInTheDocument();
  });
});

describe('auditExportParams', () => {
  it('carries actor, severities and the column selection to the signed export', () => {
    const params = auditExportParams('csv', '', '', { from: null, to: null }, null, '', 'contains', {
      actor: ' alice ',
      severities: ['warning', 'error'],
      fields: ['id', 'action'],
    });
    expect(params).toEqual({ format: 'csv', actor: 'alice', severity: 'warning,error', fields: 'id,action' });
  });

  it('omits empty selections so the export stays the full trail', () => {
    expect(
      auditExportParams('json', '', '', { from: null, to: null }, null, '', 'contains', {
        actor: ' ',
        severities: [],
        fields: [],
      }),
    ).toEqual({
      format: 'json',
    });
  });
});

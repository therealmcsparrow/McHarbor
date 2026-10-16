// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// @vitest-environment jsdom

import { act } from 'react';
import { createRoot } from 'react-dom/client';
import { describe, expect, it, vi } from 'vitest';
import { AuditEntryDetails, type AuditEntry } from '@resources/components/AuditEntryDetails';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

const baseEntry: AuditEntry = {
  id: 'audit-1',
  userId: 'user-1',
  username: 'carlo',
  action: 'user.updated',
  severity: 'success',
  entityType: 'user',
  entityId: 'user-1',
  entityName: 'carlo',
  details: 'updated user',
  ipAddress: '127.0.0.1',
  environmentId: null,
  environmentName: null,
  timestamp: '2026-08-25T09:00:00Z',
  createdAt: '2026-08-25T09:00:00Z',
  updatedAt: '2026-08-25T09:00:00Z',
};

describe('AuditEntryDetails', () => {
  it('renders structured state diff panels', () => {
    const host = document.createElement('div');
    document.body.appendChild(host);
    const root = createRoot(host);
    act(() => {
      root.render(
        <AuditEntryDetails
          entry={{
            ...baseEntry,
            changedKeys: ['name'],
            stateBefore: { name: 'Old name' },
            stateAfter: { name: 'New name' },
          }}
          envName={() => 'Local'}
        />,
      );
    });

    expect(host.textContent).toContain('audit.diffTitle');
    expect(host.textContent).toContain('name');
    expect(host.textContent).toContain('Old name');
    expect(host.textContent).toContain('New name');
    act(() => root.unmount());
    host.remove();
  });
});

describe('AuditEntryDetails API key attribution', () => {
  function render(entry: AuditEntry) {
    const host = document.createElement('div');
    document.body.appendChild(host);
    const root = createRoot(host);
    act(() => {
      root.render(<AuditEntryDetails entry={entry} envName={() => 'Local'} />);
    });
    const text = host.textContent ?? '';
    act(() => root.unmount());
    host.remove();
    return text;
  }

  it('names the API key a change was made with', () => {
    const text = render({ ...baseEntry, apiKeyId: 'key-1', apiKeyName: 'ci-deploy' });
    expect(text).toContain('audit.columnApiKey');
    expect(text).toContain('ci-deploy');
  });

  it('shows no key row for a browser-session change', () => {
    expect(render(baseEntry)).not.toContain('audit.columnApiKey');
  });
});

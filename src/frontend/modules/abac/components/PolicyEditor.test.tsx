// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { PolicyEditorDialog } from './PolicyEditorDialog';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const reference = {
  resourceTypes: [
    {
      type: 'container',
      label: 'Container',
      category: 'containers',
      source: 'host',
      resolvable: true,
      routes: [],
      attributes: [{ path: 'resource.image', type: 'string', description: 'Image' }],
    },
    {
      type: 'virtual_machine',
      label: 'Virtual machine',
      category: 'vm',
      source: 'addon-kvm',
      resolvable: true,
      routes: [],
      attributes: [{ path: 'resource.attrs.vcpus', type: 'number', description: 'Virtual CPUs' }],
    },
  ],
  variables: [{ path: 'user.id', type: 'string', description: "Caller's user id" }],
  helpers: [{ name: 'inGroup', signature: 'inGroup(name)', description: 'x', example: 'inGroup("oncall")' }],
};

const catalog = [
  { key: 'vm.view', category: 'vm', action: 'view', description: 'View virtual machines', scope: 'environment' },
  { key: 'vm.manage', category: 'vm', action: 'manage', description: 'Manage virtual machines', scope: 'environment' },
];

describe('Policy editor', () => {
  beforeEach(() => {
    grantPermissions(['rbac.policies.view', 'rbac.policies.manage', 'roles.view']);
    server.use(
      http.get('*/api/rbac/policy-reference', () => HttpResponse.json({ success: true, data: reference })),
      http.get('*/api/auth/permissions/catalog', () => HttpResponse.json({ success: true, data: catalog })),
    );
  });

  it('targets a plugin resource type, picks permissions like the roles form, and builds the expression from the reference', async () => {
    const onSubmit = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(
      <PolicyEditorDialog
        open
        onOpenChange={() => undefined}
        initial={null}
        onSubmit={onSubmit}
        isSubmitting={false}
      />,
    );

    await user.type(screen.getByLabelText('Name'), 'Big VMs');
    await user.click(screen.getByRole('button', { name: 'Resource type' }));
    await user.click(await screen.findByRole('button', { name: 'Virtual machine · addon-kvm' }));
    expect(await screen.findByText('resource.attrs.vcpus')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Include all' }));

    const expr = screen.getByRole('textbox', { name: 'Expression' }) as HTMLTextAreaElement;
    await user.clear(expr);
    await user.click(screen.getByRole('button', { name: /resource\.attrs\.vcpus/ }));
    await user.type(expr, ' > 8 && !');
    await user.click(screen.getByRole('tab', { name: 'Functions' }));
    await user.click(screen.getByRole('button', { name: /inGroup\(name\)/ }));

    await user.click(screen.getByRole('button', { name: 'Save policy' }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledTimes(1));
    const saved = onSubmit.mock.calls[0]?.[0];
    expect(saved.resourceType).toBe('virtual_machine');
    expect([...saved.permissions].sort((a: string, b: string) => a.localeCompare(b))).toEqual(['vm.manage', 'vm.view']);
    expect(saved.expression).toBe('resource.attrs.vcpus > 8 && !inGroup("oncall")');
  });

  it('replaces the expression with a chosen example', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <PolicyEditorDialog
        open
        onOpenChange={() => undefined}
        initial={null}
        onSubmit={() => undefined}
        isSubmitting={false}
      />,
    );
    await user.click(await screen.findByRole('tab', { name: 'Examples' }));
    await user.click(screen.getByRole('button', { name: /Only during office hours/ }));
    expect((screen.getByRole('textbox', { name: 'Expression' }) as HTMLTextAreaElement).value).toBe(
      '!isWeekend() && between(now.hour, 8, 18)',
    );
  });
});

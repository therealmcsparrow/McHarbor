// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ContainerInfo } from '@core/types/docker';
import { PermissionRow } from '@resources/components/security/PermissionEditor.parts';
import { ContainerActionsCell } from './ContainerActionsCell';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';

const OPERATOR = ['containers.view', 'containers.manage', 'containers.delete', 'terminal.access'];

function container(overrides: Partial<ContainerInfo>): ContainerInfo {
  return {
    Id: 'c1',
    Names: ['/harbor'],
    Image: 'registry.local/ops/harbor:9',
    ImageID: 'sha256:1',
    Command: '',
    Created: 0,
    Ports: [],
    Labels: {},
    State: 'running',
    Status: 'Up',
    NetworkSettings: { Networks: {} },
    Mounts: [],
    ...overrides,
  } as ContainerInfo;
}

function mount(c: ContainerInfo) {
  return renderWithProviders(
    <ContainerActionsCell
      container={c}
      onAction={vi.fn()}
      onTerminal={vi.fn()}
      onLogs={vi.fn()}
      onRename={vi.fn()}
      onMove={vi.fn()}
      onRecreate={vi.fn()}
      onRemove={vi.fn()}
    />,
  );
}

function hostSelfButtons() {
  return screen.queryAllByRole('button', { name: /host_self\.manage/ });
}

describe('ContainerActionsCell on McHarbor containers', () => {
  beforeEach(() => grantPermissions(OPERATOR));

  it('disables every action on McHarbor itself and names host_self.manage', () => {
    mount(container({ SelfTarget: 'app', Protected: true }));
    const gated = hostSelfButtons();
    const names = gated.map((b) => b.getAttribute('aria-label') ?? '');
    for (const action of ['Stop', 'Restart', 'Recreate', 'Rename', 'Move', 'Terminal', 'Remove']) {
      expect(names.some((n) => n.startsWith(action))).toBe(true);
    }
    for (const button of gated) expect(button).toBeDisabled();
  });

  it("names host_self.manage for a shell into a container that reaches McHarbor's host, and only for the shell", () => {
    mount(container({ Names: ['/portainer-agent'], Image: 'portainer/agent', HostAccess: ['docker_socket'] }));
    const gated = hostSelfButtons();
    expect(gated).toHaveLength(1);
    const [shell] = gated;
    expect(shell?.getAttribute('aria-label')).toMatch(/^Terminal/);
    expect(shell).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Stop' })).toBeEnabled();
  });

  it('lets a host_self.manage holder past the gate', () => {
    grantPermissions([...OPERATOR, 'host_self.manage']);
    mount(container({ Names: ['/portainer-agent'], Image: 'portainer/agent', HostAccess: ['docker_socket'] }));
    expect(hostSelfButtons()).toHaveLength(0);
    expect(screen.getByRole('button', { name: 'Terminal' })).toBeEnabled();
  });

  it('leaves an ordinary container alone', () => {
    mount(container({ Names: ['/web'], Image: 'nginx:1.27' }));
    expect(hostSelfButtons()).toHaveLength(0);
    expect(screen.getByRole('button', { name: 'Remove' })).toBeEnabled();
  });
});

describe('role editor note on containers.manage', () => {
  it("warns that containers.manage is admin-equivalent on McHarbor's own environment", () => {
    grantPermissions(['roles.manage']);
    const entry = {
      key: 'containers.manage',
      category: 'containers',
      action: 'manage',
      description: 'Manage containers',
      scope: 'environment' as const,
    };
    renderWithProviders(
      <ul>
        <PermissionRow entry={entry} state={undefined} onChange={vi.fn()} />
      </ul>,
    );
    expect(screen.getByText(/admin-equivalent/)).toHaveTextContent('host_self.manage');
  });
});

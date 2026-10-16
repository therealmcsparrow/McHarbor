// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useAgentInventory } from '@resources/hooks/useAgentInventory';
import { AgentReleasesCard } from './AgentReleasesCard';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

function InventoryProbe() {
  useAgentInventory();
  return null;
}

const refusal =
  'no release source could provide this version: https://github.com/therealmcsparrow/mcharbor-agent: SHA256SUMS signature does not verify against any pinned agent signing key';

const release = {
  build: '',
  sourceKind: 'github',
  sourceUrl: 'https://github.com/therealmcsparrow/mcharbor-agent',
  signatureKey: '',
  assets: [],
  sizeBytes: 0,
  imageRef: '',
  imageError: '',
  lastError: '',
  pulledAt: '',
  pulledBy: '',
  isTarget: false,
  inUse: false,
};

const overview = {
  target: '2026.09.02',
  state: { latestVersion: '2026.09.04', lastCheckedAt: '2026-09-27T05:00:00Z' },
  settings: {
    enabled: true,
    mode: 'notify',
    checkCron: '0 5 * * *',
    keep: 3,
    sources: [
      {
        kind: 'github',
        url: 'https://github.com/therealmcsparrow/mcharbor-agent',
      },
    ],
    imageRepository: 'ghcr.io/therealmcsparrow/mcharbor-agent',
    imagePlatforms: ['linux/amd64'],
  },
  releases: [
    {
      ...release,
      id: 'r3',
      version: '2026.09.03',
      status: 'failed',
      signatureStatus: 'refused',
      lastError: refusal,
    },
    {
      ...release,
      id: 'r2',
      version: '2026.09.02',
      build: '202609281200',
      status: 'ready',
      signatureStatus: 'verified',
      signatureKey: 'a1b2c3d4e5f60718',
      isTarget: true,
      assets: [
        {
          name: 'mcharbor-agent-linux-amd64',
          kind: 'binary',
          os: 'linux',
          arch: 'amd64',
          sha256: 'x',
          size: 20000000,
        },
      ],
    },
    {
      ...release,
      id: 'r1',
      version: '2026.09.01',
      status: 'ready',
      signatureStatus: 'verified',
      signatureKey: 'a1b2c3d4e5f60718',
    },
  ],
};

describe('AgentReleasesCard', () => {
  const targets = vi.fn();
  const pulls = vi.fn();
  const pullBuilds = vi.fn();
  const deletes = vi.fn();
  const deleteBuilds = vi.fn();
  const inventoryReads = vi.fn();

  beforeEach(() => {
    targets.mockReset();
    pulls.mockReset();
    pullBuilds.mockReset();
    deletes.mockReset();
    deleteBuilds.mockReset();
    inventoryReads.mockReset();
    grantPermissions(['updates.view', 'updates.manage']);
    server.use(
      http.get('*/api/table-views/settings-agent-releases', () =>
        HttpResponse.json({
          success: true,
          data: { layout: null, filters: [] },
        }),
      ),
      http.get('*/api/updates/agents/releases', () => HttpResponse.json({ success: true, data: overview })),
      http.put('*/api/updates/agents/releases/target', async ({ request }) => {
        targets(await request.json());
        return HttpResponse.json({ success: true, data: {} });
      }),
      http.post('*/api/updates/agents/releases/:version/pull', ({ params, request }) => {
        pulls(params.version);
        pullBuilds(new URL(request.url).searchParams.get('build'));
        return HttpResponse.json({ success: true, data: {} });
      }),
      http.delete('*/api/updates/agents/releases/:version', ({ params, request }) => {
        deletes(params.version);
        deleteBuilds(new URL(request.url).searchParams.get('build'));
        return HttpResponse.json({ success: true, data: {} });
      }),
    );
  });

  it('marks a pinned build so the operator knows checks will leave it alone', async () => {
    server.use(
      http.get('*/api/updates/agents/releases', () =>
        HttpResponse.json({
          success: true,
          data: {
            ...overview,
            releases: [
              {
                ...release,
                id: 'p1',
                version: '2026.10.01',
                build: '202610050818',
                status: 'ready',
                signatureStatus: 'verified',
                pinned: true,
              },
            ],
          },
        }),
      ),
    );
    renderWithProviders(<AgentReleasesCard />);
    const row = await screen.findByTestId('agent-release-2026.10.01-202610050818');
    expect(within(row).getByText('Pinned')).toBeInTheDocument();
  });

  it('keeps a refused release behind the error icon rather than in the row', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    const row = await screen.findByTestId('agent-release-2026.09.03');
    expect(within(row).getByText('Refused')).toBeInTheDocument();
    expect(within(row).queryByText(refusal)).not.toBeInTheDocument();
    await user.click(within(row).getByRole('button', { name: 'View the error' }));
    expect(await screen.findByText(refusal)).toBeInTheDocument();
  });

  it('makes a stored release the target, and never a refused one or the current target', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    const stored = await screen.findByTestId('agent-release-2026.09.01');
    expect(
      within(screen.getByTestId('agent-release-2026.09.03')).getByRole('button', {
        name: 'Make 2026.09.03 the target',
      }),
    ).toBeDisabled();
    expect(
      within(screen.getByTestId('agent-release-2026.09.02-202609281200')).getByRole('button', {
        name: 'Make 2026.09.02 (202609281200) the target',
      }),
    ).toBeDisabled();
    await user.click(
      within(stored).getByRole('button', {
        name: 'Make 2026.09.01 the target',
      }),
    );
    await waitFor(() => expect(targets).toHaveBeenCalledTimes(1));
    expect(targets).toHaveBeenCalledWith({ version: '2026.09.01' });
  });

  it('deletes only after confirmation, and never the target', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    const target = await screen.findByTestId('agent-release-2026.09.02-202609281200');
    expect(within(target).getByRole('button', { name: 'Delete 2026.09.02 (202609281200)' })).toBeDisabled();
    await user.click(
      within(screen.getByTestId('agent-release-2026.09.01')).getByRole('button', { name: 'Delete 2026.09.01' }),
    );
    const dialog = await screen.findByRole('dialog');
    expect(deletes).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    expect(deletes).toHaveBeenCalledWith('2026.09.01');
  });

  it('offers the newest release the check found when it is not stored', async () => {
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    await user.click(await screen.findByRole('button', { name: 'Download 2026.09.04' }));
    await waitFor(() => expect(pulls).toHaveBeenCalledWith('2026.09.04'));
  });

  it('offers a newer build of the stored target, and nothing once that build is stored', async () => {
    const withState = (latestBuild: string) =>
      http.get('*/api/updates/agents/releases', () =>
        HttpResponse.json({
          success: true,
          data: { ...overview, state: { latestVersion: '2026.09.02', latestBuild } },
        }),
      );
    server.use(withState('202610010900'));
    const user = userEvent.setup();
    const { unmount } = renderWithProviders(<AgentReleasesCard />);
    await user.click(await screen.findByRole('button', { name: 'Download 2026.09.02 (202610010900)' }));
    await waitFor(() => expect(pulls).toHaveBeenCalledWith('2026.09.02'));
    // Named explicitly: a build-less pull leaves a pinned older build in place.
    expect(pullBuilds).toHaveBeenCalledWith('202610010900');
    unmount();

    server.use(withState('202609281200'));
    renderWithProviders(<AgentReleasesCard />);
    await screen.findByTestId('agent-release-2026.09.02-202609281200');
    expect(screen.queryByRole('button', { name: /^Download 2026.09.02/ })).not.toBeInTheDocument();
  });

  it('keeps both builds of one version and acts on the one picked', async () => {
    const twoBuilds = {
      ...overview,
      target: '2026.09.02',
      targetBuild: '202610010900',
      releases: [
        {
          ...release,
          id: 'n',
          version: '2026.09.02',
          build: '202610010900',
          status: 'ready',
          signatureStatus: 'verified',
          isTarget: true,
        },
        {
          ...release,
          id: 'o',
          version: '2026.09.02',
          build: '202609281200',
          status: 'ready',
          signatureStatus: 'verified',
        },
      ],
    };
    server.use(http.get('*/api/updates/agents/releases', () => HttpResponse.json({ success: true, data: twoBuilds })));
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    expect(await screen.findByText('Agents are served 2026.09.02 (202610010900).')).toBeInTheDocument();
    const old = await screen.findByTestId('agent-release-2026.09.02-202609281200');
    expect(screen.getByTestId('agent-release-2026.09.02-202610010900')).toBeInTheDocument();

    await user.click(within(old).getByRole('button', { name: 'Make 2026.09.02 (202609281200) the target' }));
    await waitFor(() => expect(targets).toHaveBeenCalledTimes(1));
    expect(targets).toHaveBeenCalledWith({ version: '2026.09.02', build: '202609281200' });

    await user.click(within(old).getByRole('button', { name: 'Delete 2026.09.02 (202609281200)' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(deletes).toHaveBeenCalledTimes(1));
    expect(deleteBuilds).toHaveBeenCalledWith('202609281200');
  });

  it('re-reads the agents table when the polled target moves to a downloaded build', async () => {
    let current = { ...overview, targetBuild: '202609281200' };
    server.use(
      http.get('*/api/updates/agents/releases', () => HttpResponse.json({ success: true, data: current })),
      http.get('*/api/updates/agents', () => {
        inventoryReads();
        return HttpResponse.json({ success: true, data: { agents: [], targetVersion: '2026.09.02' } });
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(
      <>
        <AgentReleasesCard />
        <InventoryProbe />
      </>,
    );
    await screen.findByTestId('agent-release-2026.09.02-202609281200');
    await waitFor(() => expect(inventoryReads).toHaveBeenCalledTimes(1));

    current = { ...overview, targetBuild: '202610010900' };
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(inventoryReads).toHaveBeenCalledTimes(2));
  });

  it('renders the actions disabled, naming updates.manage, when it is missing', async () => {
    grantPermissions(['updates.view']);
    const user = userEvent.setup();
    renderWithProviders(<AgentReleasesCard />);
    const row = await screen.findByTestId('agent-release-2026.09.01');
    const button = within(row).getByRole('button', {
      name: 'Make 2026.09.01 the target',
    });
    expect(button).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Download 2026.09.04' })).toBeDisabled();
    await user.hover(button.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the updates.manage permission')).not.toHaveLength(0);
    expect(targets).not.toHaveBeenCalled();
  });
});

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import type { HostFSStatus } from '@core/types/docker';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { HostDiskStatus } from './HostDiskStatus';

const GIB = 1024 ** 3;

function levelFor(percent: number, t: { warning: number; critical: number }): HostFSStatus['level'] {
  if (percent >= t.critical) return 'critical';
  if (percent >= t.warning) return 'warning';
  return 'normal';
}

// Mirrors the live route: ok carries level and lastSeen and no reason; a degraded state
// carries a reason and level null; thresholds are never 0.
function status(env: string, over: Partial<HostFSStatus>): HostFSStatus {
  const state = over.state ?? 'ok';
  const percent = over.percent ?? 50;
  const thresholds = over.thresholds ?? { warning: 85, critical: 95 };
  const ok = state === 'ok';
  const reason = ok || state === 'unsupported' ? {} : { reason: 'agent_offline' as const };
  return {
    environmentId: env,
    state,
    ...reason,
    level: ok ? levelFor(percent, thresholds) : null,
    path: '/',
    totalBytes: 100 * GIB,
    usedBytes: (percent / 100) * 100 * GIB,
    percent,
    lastSeen: state === 'unknown' || state === 'unsupported' ? null : new Date().toISOString(),
    thresholds,
    ...over,
  };
}

function serve(byEnv: Record<string, HostFSStatus>, seen: string[] = []) {
  server.use(
    http.get('*/api/metrics/host/fs', ({ request }) => {
      const env = new URL(request.url).searchParams.get('env') ?? '';
      seen.push(env);
      const row = byEnv[env];
      return row
        ? HttpResponse.json({ success: true, data: row })
        : HttpResponse.json({ success: false, error: 'not found' }, { status: 404 });
    }),
  );
}

describe('HostDiskStatus', () => {
  it('shows a grey worded state and no bar when the disk has no data, never a green bar', async () => {
    grantPermissions(['host.view']);
    const lastSeen = new Date(Date.now() - 3 * 3600 * 1000).toISOString();
    serve({ e1: status('e1', { state: 'stale', lastSeen }) });
    renderWithProviders(<HostDiskStatus envId="e1" />);
    expect(await screen.findByText(/No disk data, last seen/)).toBeInTheDocument();
    expect(screen.getByText(/3 hours ago/)).toBeInTheDocument();
    expect(screen.queryByRole('meter')).not.toBeInTheDocument();
  });

  it('says so when a host was never measured, and when it is not monitored', async () => {
    grantPermissions(['host.view']);
    serve({
      e1: status('e1', { state: 'unknown', percent: 0, totalBytes: 0, usedBytes: 0 }),
      e2: status('e2', { state: 'unsupported', percent: 0, totalBytes: 0, usedBytes: 0 }),
    });
    renderWithProviders(
      <>
        <HostDiskStatus envId="e1" />
        <HostDiskStatus envId="e2" />
      </>,
    );
    expect(await screen.findByText('No disk data yet')).toBeInTheDocument();
    expect(await screen.findByText('Not monitored')).toBeInTheDocument();
    expect(screen.queryByRole('meter')).not.toBeInTheDocument();
  });

  it('without a server level, colours from the reported thresholds and falls back to 85/95 when they are 0', async () => {
    grantPermissions(['host.view']);
    serve({
      a: status('a', { percent: 70, level: null, thresholds: { warning: 60, critical: 90 } }),
      b: status('b', { percent: 70, level: null, thresholds: { warning: 0, critical: 0 } }),
      c: status('c', { percent: 96, level: null, thresholds: { warning: 60, critical: 90 } }),
    });
    renderWithProviders(
      <>
        <HostDiskStatus envId="a" />
        <HostDiskStatus envId="b" />
        <HostDiskStatus envId="c" />
      </>,
    );
    await screen.findByText(/above critical threshold/);
    const bars = await screen.findAllByRole('meter');
    expect(bars).toHaveLength(3);
    const fills = bars.map((bar) => bar.nextElementSibling?.firstElementChild?.className ?? '');
    expect(fills[0]).toContain('bg-warning');
    expect(fills[1]).toContain('bg-primary');
    expect(fills[2]).toContain('bg-danger');
  });

  it('uses the server level over its own calculation, so client and alert rules cannot diverge', async () => {
    grantPermissions(['host.view']);
    serve({
      a: status('a', { percent: 70, level: 'critical' }),
      b: status('b', { percent: 99, level: 'normal' }),
    });
    renderWithProviders(
      <>
        <HostDiskStatus envId="a" />
        <HostDiskStatus envId="b" />
      </>,
    );
    await screen.findByText(/above critical threshold/);
    const bars = await screen.findAllByRole('meter');
    expect(bars).toHaveLength(2);
    const fills = bars.map((bar) => bar.nextElementSibling?.firstElementChild?.className ?? '');
    expect(fills[0]).toContain('bg-danger');
    expect(fills[1]).toContain('bg-primary');
  });

  it('asks for exactly the environment it was given', async () => {
    grantPermissions(['host.view']);
    const seen: string[] = [];
    serve({ e1: status('e1', {}), e2: status('e2', { percent: 91 }) }, seen);
    renderWithProviders(<HostDiskStatus envId="e2" />);
    expect(await screen.findByRole('meter')).toHaveValue(91);
    expect(seen).toEqual(['e2']);
  });

  it('names the missing permission and sends no request without host.view', async () => {
    grantPermissions([]);
    const seen: string[] = [];
    serve({ e1: status('e1', {}) }, seen);
    renderWithProviders(<HostDiskStatus envId="e1" />);
    expect(await screen.findByText(/host\.view/)).toBeInTheDocument();
    expect(seen).toEqual([]);
  });
});

// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { ScheduleDialog } from './ScheduleDialog';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import type { ScheduleItem } from '../types';

const weekly: ScheduleItem = {
  id: 'sch-1',
  name: 'Weekly restart',
  description: 'Sunday maintenance window',
  // Sunday at 04:00. The bug this guards against is an editor that rewrites the
  // weekday to today's just by being opened.
  cron: '0 4 * * 0',
  action: 'restart',
  target: 'nginx-proxy',
  enabled: true,
  lastRunAt: '',
  nextRunAt: '',
};

// ReadableScheduleField opens in Readable mode for an expression it can
// round-trip, so the raw cron is not on screen as an input. What matters for this
// test is that the value the dialog will submit is still the one it was given,
// which the submit assertions below check directly.

describe('ScheduleDialog', () => {
  beforeEach(() => {
    grantPermissions('*');
  });

  it('opens on an existing schedule without rewriting its values', async () => {
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} schedule={weekly} />);

    expect(screen.getByDisplayValue('Weekly restart')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Sunday maintenance window')).toBeInTheDocument();
    expect(screen.getByDisplayValue('nginx-proxy')).toBeInTheDocument();
    // The schedule itself is asserted through what gets submitted, in the PUT
    // test below: a field that renders correctly but submits today's weekday is
    // the actual failure mode.
  });

  it('opens empty when creating, and offers no active switch', () => {
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} />);

    expect(screen.getByRole('button', { name: 'Create' })).toBeInTheDocument();
    expect(screen.queryByDisplayValue('Weekly restart')).not.toBeInTheDocument();
    // A create starts active; only the update route accepts the flag, so showing
    // a switch that cannot be sent would be a lie.
    expect(screen.queryByRole('switch', { name: 'Active' })).not.toBeInTheDocument();
  });

  it('shows the active switch only when editing', () => {
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} schedule={weekly} />);

    expect(screen.getByRole('switch', { name: 'Active' })).toBeInTheDocument();
  });

  it('PUTs to the schedule being edited, carrying the enabled flag', async () => {
    const seen: { url: string; method: string; body: unknown }[] = [];
    server.use(
      http.put('*/api/schedules/:id', async ({ request, params }) => {
        seen.push({ url: String(params.id), method: 'PUT', body: await request.json() });
        return HttpResponse.json({ success: true, data: { ...weekly } });
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} schedule={weekly} />);

    await user.click(screen.getByRole('switch', { name: 'Active' }));
    await user.click(screen.getByRole('button', { name: 'Save schedule' }));

    await waitFor(() => expect(seen).toHaveLength(1));
    expect(seen[0]!.url).toBe('sch-1');
    const body = seen[0]!.body as Record<string, unknown>;
    expect(body.name).toBe('Weekly restart');
    expect(body.cron).toBe('0 4 * * 0');
    // Toggled off, and sent — an edit that silently dropped the flag would make
    // the switch decorative.
    expect(body.enabled).toBe(false);
  });

  it('POSTs a new schedule and sends no enabled flag', async () => {
    const bodies: Record<string, unknown>[] = [];
    server.use(
      http.post('*/api/schedules', async ({ request }) => {
        bodies.push((await request.json()) as Record<string, unknown>);
        return HttpResponse.json({ success: true, data: { ...weekly } });
      }),
    );

    const user = userEvent.setup();
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} />);

    await user.type(screen.getByPlaceholderText('Nightly restart'), 'Nightly');
    await user.type(screen.getByPlaceholderText('nginx'), 'redis');
    await user.click(screen.getByRole('button', { name: 'Create' }));

    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(bodies[0]!.name).toBe('Nightly');
    expect(bodies[0]!.target).toBe('redis');
    expect('enabled' in bodies[0]!).toBe(false);
  });

  it('refuses to submit until name, schedule and target are all present', async () => {
    const user = userEvent.setup();
    renderWithProviders(<ScheduleDialog open onOpenChange={() => {}} />);

    const create = screen.getByRole('button', { name: 'Create' });
    expect(create).toBeDisabled();

    await user.type(screen.getByPlaceholderText('Nightly restart'), 'Nightly');
    // A name alone is not enough: a schedule with no target would be accepted by
    // the form and refused by the server.
    expect(create).toBeDisabled();

    await user.type(screen.getByPlaceholderText('nginx'), 'redis');
    expect(create).toBeEnabled();
  });
});

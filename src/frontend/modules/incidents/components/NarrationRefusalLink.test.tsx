// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter } from 'react-router';
import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { useEnvironmentStore } from '@resources/stores/environment';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import FindingsPage from '../pages/FindingsPage';
import { NarrativePanel } from './NarrativePanel';
import { TimelinePanel } from './TimelinePanel';
import type { Incident } from '@resources/hooks/useIncidents';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
// eslint-disable-next-line mcharbor/no-restricted-syntax -- unavoidable cast: jsdom has no ResizeObserver
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const open: Incident = {
  id: 'i1',
  envId: 'e1',
  subjectType: 'container',
  subjectName: 'api',
  signal: 'crash_loop',
  status: 'open',
  signalCount: 4,
  firstSignalAt: '2026-09-26T10:01:00Z',
  lastSignalAt: '2026-09-26T10:04:00Z',
  narrativeStatus: 'none',
  createdAt: '2026-09-26T10:01:00Z',
};

const refusal = () =>
  HttpResponse.json(
    { success: false, error: 'Invalid request', code: 'err.invalid_body', data: { reason: 'not_opted_in' } },
    { status: 409 },
  );

let settingsEnvs: string[] = [];

beforeEach(() => {
  grantPermissions(['incidents.view', 'incidents.manage']);
  settingsEnvs = [];
  useEnvironmentStore.setState({ currentId: 'e-current' });
  server.use(
    http.post('*/api/incidents/:id/narrate', refusal),
    http.post('*/api/incidents/:id/summarise', refusal),
    http.post('*/api/incidents/findings/:id/explain', refusal),
    http.get('*/api/incidents/settings', ({ request }) => {
      const env = new URL(request.url).searchParams.get('env') ?? '';
      settingsEnvs.push(env);
      return HttpResponse.json({
        success: true,
        data: { envId: env, narrateOptIn: false, proactive: false, hourlyCap: 10, modelConfigured: true },
      });
    }),
  );
});

async function openSettingsFrom(alert: HTMLElement) {
  const user = userEvent.setup();
  expect(within(alert).getByText(/Turn it on in the narration settings first/)).toBeInTheDocument();
  const way =
    within(alert).queryByRole('link', { name: /narration settings/i }) ??
    within(alert).queryByRole('button', { name: /narration settings/i });
  expect(way, 'the refusal names the narration settings but offers no way to reach them').not.toBeNull();
  await user.click(way as HTMLElement);
  return screen.findByRole('dialog');
}

describe('Narration refusal: not_opted_in', () => {
  it('NarrativePanel opens the settings of the incident environment, not the selected one', async () => {
    const user = userEvent.setup();
    renderWithProviders(
      <MemoryRouter initialEntries={['/incidents/i1']}>
        <NarrativePanel incident={open} />
      </MemoryRouter>,
    );
    await user.click(screen.getByRole('button', { name: 'Explain this' }));
    const dialog = await openSettingsFrom(await screen.findByRole('alert'));
    expect(await within(dialog).findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(settingsEnvs).toEqual(['e1']);
  });

  it('TimelinePanel offers the same way, for the incident environment', async () => {
    const user = userEvent.setup();
    renderWithProviders(<TimelinePanel incident={{ ...open, envId: 'e7', status: 'resolved' }} />);
    await user.click(screen.getByRole('button', { name: 'Summarise' }));
    const dialog = await openSettingsFrom(await screen.findByRole('alert'));
    expect(await within(dialog).findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(settingsEnvs).toEqual(['e7']);
  });

  function serveFindingsOf(owner: string) {
    server.use(
      http.get('*/api/incidents/findings', ({ request }) => {
        const env = new URL(request.url).searchParams.get('env');
        return HttpResponse.json({
          success: true,
          data:
            env === owner
              ? [
                  {
                    id: 'f1',
                    envId: owner,
                    subjectType: 'container',
                    subjectName: 'api',
                    kind: 'scheduled_restarts',
                    status: 'open',
                    metrics: { hour: 2, minute: 40, days: 4 },
                    firstSeenAt: '2026-09-20T00:00:00Z',
                    lastSeenAt: '2026-09-26T00:00:00Z',
                  },
                ]
              : [],
        });
      }),
    );
    if (!document.getElementById('pagehead-slot')) {
      const slot = document.createElement('div');
      slot.id = 'pagehead-slot';
      document.body.appendChild(slot);
    }
    useEnvironmentStore.setState({ currentId: owner });
  }

  const renderFindings = () =>
    renderWithProviders(
      <MemoryRouter>
        <FindingsPage />
      </MemoryRouter>,
    );

  it('FindingsPage offers the same way, for the environment of the finding that was explained', async () => {
    serveFindingsOf('e2');
    const user = userEvent.setup();
    renderFindings();
    await user.click(await screen.findByRole('button', { name: 'Explain this finding' }));
    const dialog = await openSettingsFrom(await screen.findByRole('alert'));
    expect(await within(dialog).findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(settingsEnvs).toEqual(['e2']);
  });

  it('FindingsPage keeps the refused finding environment after the selected environment changes', async () => {
    serveFindingsOf('e2');
    const user = userEvent.setup();
    renderFindings();
    await user.click(await screen.findByRole('button', { name: 'Explain this finding' }));
    const alert = await screen.findByRole('alert');
    await act(async () => useEnvironmentStore.setState({ currentId: 'e-other' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Explain this finding' })).toBeNull());
    const dialog = await openSettingsFrom(alert);
    expect(await within(dialog).findByRole('button', { name: 'Save' })).toBeInTheDocument();
    expect(settingsEnvs).toEqual(['e2']);
  });

  it('lets a viewer read the settings but keeps Save disabled, naming incidents.manage', async () => {
    const user = userEvent.setup();
    renderWithProviders(<NarrativePanel incident={open} />);
    await user.click(screen.getByRole('button', { name: 'Explain this' }));
    const alert = await screen.findByRole('alert');
    grantPermissions(['incidents.view']);
    const dialog = await openSettingsFrom(alert);
    const save = await within(dialog).findByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.hover(save.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the incidents.manage permission')).not.toHaveLength(0);
  });
});

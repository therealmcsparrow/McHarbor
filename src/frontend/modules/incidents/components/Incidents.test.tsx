// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter, Route, Routes } from 'react-router';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { NarrativePanel } from './NarrativePanel';
import { NarrationSettingsDialog } from './NarrationSettingsDialog';
import { NarrativeSnippet } from '@resources/components/incidents/NarrativeSnippet';
import IncidentDetailPage from '../pages/IncidentDetailPage';
import FindingsPage from '../pages/FindingsPage';
import type { Incident } from '@resources/hooks/useIncidents';
import type { NarrationSettings } from '../hooks/useNarrationSettings';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
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

const narrated: Incident = {
  ...open,
  narrativeStatus: 'ready',
  narrative: {
    summary: 'api has crash-looped since 10:01 after the image moved to api:2.',
    changes: ['image api:1 -> api:2'],
    causes: [
      {
        title: 'api:2 fails to start',
        evidence: ['log: panic: missing DATABASE_URL'],
        confidence: 'high',
      },
    ],
    confidence: 'medium',
    undetermined: false,
  },
  narrativeMeta: {
    provider: 'ollama',
    connection: 'home-gpu',
    model: 'llama3',
    requestedBy: 'alice',
    at: '2026-09-26T10:06:00Z',
    promptSha256: 'abcdef0123456789',
    proactive: false,
  },
  narrativePrompt: 'Incident on container "api" (signal: crash_loop)... panic: missing DATABASE_URL',
};

describe('Narration', () => {
  const narrations = vi.fn();
  beforeEach(() => {
    narrations.mockReset();
    grantPermissions(['incidents.view', 'incidents.manage']);
  });

  it('says why nothing was sent when the environment has not opted in', async () => {
    server.use(
      http.post('*/api/incidents/:id/narrate', () => {
        narrations();
        return HttpResponse.json(
          {
            success: false,
            error: 'Invalid request',
            code: 'err.invalid_body',
            data: { reason: 'not_opted_in' },
          },
          { status: 409 },
        );
      }),
    );
    const user = userEvent.setup();
    renderWithProviders(<NarrativePanel incident={open} />);
    await user.click(screen.getByRole('button', { name: 'Explain this' }));
    expect(await screen.findByText(/Narration is off for this environment/)).toBeInTheDocument();
    expect(narrations).toHaveBeenCalledTimes(1);
  });

  it('shows the causes with their evidence, who asked which model, and exactly what the model was shown', async () => {
    const user = userEvent.setup();
    renderWithProviders(<NarrativePanel incident={narrated} />);
    expect(screen.getByText('api:2 fails to start')).toBeInTheDocument();
    expect(screen.getByText('log: panic: missing DATABASE_URL')).toBeInTheDocument();
    expect(screen.getByText(/Written by llama3 through home-gpu \(ollama\) for alice/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /What the model was shown/ }));
    expect(await screen.findByText(/Incident on container "api"/)).toBeInTheDocument();
  });

  it('says plainly when the model could not determine a cause', () => {
    renderWithProviders(
      <NarrativePanel
        incident={{
          ...narrated,
          narrative: {
            ...narrated.narrative!,
            causes: [],
            undetermined: true,
            confidence: 'low',
          },
        }}
      />,
    );
    expect(screen.getByText(/could not determine a cause/)).toBeInTheDocument();
    expect(screen.getByText('Low confidence')).toBeInTheDocument();
  });

  it('is disabled, naming incidents.manage, for a viewer, and asks once on a double click', async () => {
    grantPermissions(['incidents.view']);
    const user = userEvent.setup();
    const { unmount } = renderWithProviders(<NarrativePanel incident={open} />);
    const explain = screen.getByRole('button', { name: 'Explain this' });
    expect(explain).toBeDisabled();
    await user.hover(explain.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the incidents.manage permission')).not.toHaveLength(0);
    unmount();

    grantPermissions(['incidents.view', 'incidents.manage']);
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/incidents/:id/narrate', async () => {
        narrations();
        await gate;
        return HttpResponse.json({ success: true, data: narrated });
      }),
    );
    renderWithProviders(<NarrativePanel incident={open} />);
    const button = screen.getByRole('button', { name: 'Explain this' });
    await user.dblClick(button);
    await user.click(button);
    release();
    await waitFor(() => expect(narrations).toHaveBeenCalledTimes(1));
  });
});

const settings: NarrationSettings = {
  envId: 'e1',
  narrateOptIn: false,
  proactive: false,
  hourlyCap: 10,
  modelConfigured: true,
  model: {
    provider: 'anthropic',
    connection: 'vendor',
    model: 'claude',
    baseUrl: 'https://api.example',
    local: false,
  },
};

describe('Narration opt-in', () => {
  const saves = vi.fn();
  beforeEach(() => {
    saves.mockReset();
    grantPermissions(['incidents.view', 'incidents.manage']);
    server.use(
      http.get('*/api/incidents/settings', () => HttpResponse.json({ success: true, data: settings })),
      http.put('*/api/incidents/settings', async ({ request }) => {
        saves(await request.json());
        return HttpResponse.json({
          success: true,
          data: { ...settings, narrateOptIn: true },
        });
      }),
    );
  });

  it('names where the evidence goes, and saves only after the operator acknowledges it', async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    renderWithProviders(<NarrationSettingsDialog envId="e1" onOpenChange={onOpenChange} />);
    const dialog = await screen.findByRole('dialog');
    expect(
      await within(dialog).findByText(/Evidence would leave this network: it goes to vendor, anthropic running claude/),
    ).toBeInTheDocument();
    await user.click(
      within(dialog).getByRole('switch', {
        name: 'Allow narration for this environment',
      }),
    );
    const save = within(dialog).getByRole('button', { name: 'Save' });
    expect(save).toBeDisabled();
    await user.click(within(dialog).getByRole('checkbox'));
    await user.click(save);
    await waitFor(() => expect(saves).toHaveBeenCalledTimes(1));
    expect(saves.mock.calls[0]?.[0]).toMatchObject({
      envId: 'e1',
      narrateOptIn: true,
      proactive: false,
      hourlyCap: 10,
    });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });
});

describe('Resolving an incident', () => {
  const resolves = vi.fn();
  function mount() {
    if (!document.getElementById('pagehead-slot')) {
      const slot = document.createElement('div');
      slot.id = 'pagehead-slot';
      document.body.appendChild(slot);
    }
    renderWithProviders(
      <MemoryRouter initialEntries={['/incidents/i1']}>
        <Routes>
          <Route path="/incidents/:id" element={<IncidentDetailPage />} />
        </Routes>
      </MemoryRouter>,
    );
  }
  beforeEach(() => {
    resolves.mockReset();
    grantPermissions(['incidents.view', 'incidents.manage']);
    server.use(http.get('*/api/incidents/i1', () => HttpResponse.json({ success: true, data: open })));
  });

  it('resolves only after confirmation, once, and keeps the dialog with the server message on failure', async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server.use(
      http.post('*/api/incidents/:id/resolve', async () => {
        resolves();
        await gate;
        return HttpResponse.json({ success: false, error: 'building timeline: database is locked' }, { status: 500 });
      }),
    );
    const user = userEvent.setup();
    mount();
    await user.click(await screen.findByRole('button', { name: 'Resolve' }));
    expect(resolves).not.toHaveBeenCalled();
    const confirm = screen.getByRole('button', { name: 'Resolve incident' });
    await user.dblClick(confirm);
    await user.click(confirm);
    release();
    expect(await screen.findByText('building timeline: database is locked')).toBeInTheDocument();
    expect(resolves).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: 'Resolve incident' })).toBeInTheDocument();
  });

  it('is disabled, naming incidents.manage, for a viewer', async () => {
    grantPermissions(['incidents.view']);
    const user = userEvent.setup();
    mount();
    const resolve = await screen.findByRole('button', { name: 'Resolve' });
    expect(resolve).toBeDisabled();
    await user.hover(resolve.parentElement as HTMLElement);
    expect(await screen.findAllByText('Requires the incidents.manage permission')).not.toHaveLength(0);
  });
});

describe('Narrative on the on-call inbox', () => {
  function mount() {
    renderWithProviders(
      <MemoryRouter>
        <NarrativeSnippet alertEventId="ev1" />
      </MemoryRouter>,
    );
  }

  it('shows the narrative of the incident the alert belongs to', async () => {
    grantPermissions(['incidents.view']);
    server.use(http.get('*/api/incidents/by-alert/ev1', () => HttpResponse.json({ success: true, data: narrated })));
    mount();
    expect(await screen.findByText(/crash-looped since 10:01/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open the incident' })).toHaveAttribute('href', '/incidents/i1');
  });

  it('asks nothing without incidents.view', async () => {
    const asked = vi.fn();
    grantPermissions(['alerts.view']);
    server.use(
      http.get('*/api/incidents/by-alert/ev1', () => {
        asked();
        return HttpResponse.json({ success: true, data: narrated });
      }),
    );
    mount();
    await new Promise((r) => setTimeout(r, 50));
    expect(asked).not.toHaveBeenCalled();
    expect(screen.queryByText(/crash-looped/)).toBeNull();
  });
});

describe('Behaviour findings', () => {
  it('states the detector’s numbers in words, without asking a model', async () => {
    grantPermissions(['incidents.view']);
    server.use(
      http.get('*/api/incidents/findings', () =>
        HttpResponse.json({
          success: true,
          data: [
            {
              id: 'f1',
              envId: 'e1',
              subjectType: 'container',
              subjectName: 'api',
              kind: 'scheduled_restarts',
              status: 'open',
              metrics: { hour: 2, minute: 40, days: 4 },
              firstSeenAt: '2026-09-20T00:00:00Z',
              lastSeenAt: '2026-09-26T00:00:00Z',
            },
          ],
        }),
      ),
    );
    if (!document.getElementById('pagehead-slot')) {
      const slot = document.createElement('div');
      slot.id = 'pagehead-slot';
      document.body.appendChild(slot);
    }
    renderWithProviders(
      <MemoryRouter>
        <FindingsPage />
      </MemoryRouter>,
    );
    expect(await screen.findByText('Restarts around 02:40 UTC on 4 different days.')).toBeInTheDocument();
  });
});

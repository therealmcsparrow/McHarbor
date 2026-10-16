// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MemoryRouter, Route, Routes } from 'react-router';
import { screen, within } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it } from 'vitest';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import IncidentDetailPage from '../pages/IncidentDetailPage';
import type { Incident } from '@resources/hooks/useIncidents';

class NoopResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= NoopResizeObserver as unknown as typeof ResizeObserver;

const vmIncident: Incident = {
  id: 'p1',
  envId: 'e1',
  subjectType: 'virtual_machine',
  subjectName: 'db-vm',
  subjectId: 'vm-1',
  source: 'addon-kvm',
  signal: 'vm_crashed',
  status: 'open',
  signalCount: 1,
  firstSignalAt: '2026-09-27T10:00:00Z',
  lastSignalAt: '2026-09-27T10:00:00Z',
  narrativeStatus: 'none',
  createdAt: '2026-09-27T10:00:00Z',
  evidence: {
    windowStart: '2026-09-27T09:30:00Z',
    windowEnd: '2026-09-27T10:05:00Z',
    container: { name: '' },
    events: [],
    logs: [],
    imageChanges: [],
    audit: [],
    deploys: [],
    alerts: [],
    hostCpu: [],
    hostMem: [],
    containerCpu: [],
    containerMem: [],
    drift: [],
    subject: {
      type: 'virtual_machine',
      id: 'vm-1',
      name: 'db-vm',
      source: 'addon-kvm',
      facts: { state: 'crashed', vcpus: '4' },
    },
    sections: [
      { title: 'QEMU unit log', kind: 'log', lines: ['qemu-system-x86_64: terminating on signal 9'] },
      { title: 'Disks', kind: 'table', columns: ['Slot', 'Pool'], rows: [['vda', 'fast-ssd']] },
      { title: 'vTPM unit log', kind: 'text', note: 'plugin_unavailable' },
    ],
  },
};

function mount() {
  if (!document.getElementById('pagehead-slot')) {
    const slot = document.createElement('div');
    slot.id = 'pagehead-slot';
    document.body.appendChild(slot);
  }
  renderWithProviders(
    <MemoryRouter initialEntries={['/incidents/p1']}>
      <Routes>
        <Route path="/incidents/:id" element={<IncidentDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('Incidents raised by a plugin', () => {
  beforeEach(() => {
    grantPermissions(['incidents.view', 'incidents.manage']);
    server.use(
      http.get('*/api/incidents/p1', () => HttpResponse.json({ success: true, data: vmIncident })),
      http.get('*/api/incidents/sources', () =>
        HttpResponse.json({
          success: true,
          data: [
            {
              source: 'addon-kvm',
              subjectTypes: [
                {
                  type: 'virtual_machine',
                  label: 'Virtual machine',
                  signals: [{ key: 'vm_crashed', label: 'VM crashed', strength: 5 }],
                },
              ],
            },
          ],
        }),
      ),
    );
  });

  it("shows the plugin's facts and sections instead of container evidence, labelled as the plugin declared", async () => {
    mount();
    expect(await screen.findByText('qemu-system-x86_64: terminating on signal 9')).toBeInTheDocument();
    expect(screen.getByText('crashed')).toBeInTheDocument();
    const table = screen.getByRole('table');
    expect(within(table).getByText('fast-ssd')).toBeInTheDocument();
    expect(screen.getByText('The plugin that raised this incident did not answer in time.')).toBeInTheDocument();
    expect(screen.queryByText('Image changes')).not.toBeInTheDocument();
    expect(screen.queryByText('Deploys to this environment')).not.toBeInTheDocument();
    expect(await screen.findByText(/VM crashed/)).toBeInTheDocument();
  });
});

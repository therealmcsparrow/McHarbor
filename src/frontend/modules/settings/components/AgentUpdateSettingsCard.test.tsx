// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { fireEvent, screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { PERM } from '@core/auth/permissionKeys';
import { grantPermissions } from '../../../test/permissions';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { AgentUpdateSettingsCard } from './AgentUpdateSettingsCard';

describe('AgentUpdateSettingsCard', () => {
  it('reports the queued sweep after "Update agents now" instead of crashing', async () => {
    grantPermissions([PERM.ENVIRONMENTS_MANAGE]);
    server.use(
      http.get('*/api/updates/settings', () =>
        HttpResponse.json({
          success: true,
          data: {
            intervalHours: 24,
            channelIds: [],
            lastSeenVersion: '',
            enabled: true,
            autoInstall: false,
            autoUpdateAgents: false,
            agentExcludedEnvIds: [],
            checkCron: '0 3 * * *',
            installCron: '0 4 * * *',
            installWindowMinutes: 60,
          },
        }),
      ),
      http.get('*/api/updates/state', () =>
        HttpResponse.json({ success: true, data: { latestAgentVersion: '2026.10.01' } }),
      ),
      // What the handler returns now: the sweep is started, not awaited.
      http.post('*/api/updates/agents/update', () =>
        HttpResponse.json({
          success: true,
          data: {
            message: 'Agent update triggered',
            code: 'msg.agent_update_triggered',
            queued: 2,
            targetVersion: '2026.10.01',
          },
        }),
      ),
    );

    renderWithProviders(<AgentUpdateSettingsCard />);
    fireEvent.click(await screen.findByRole('button', { name: 'Update agents now' }));

    expect(
      await screen.findByText('2 agents queued for 2026.10.01; the table shows their progress.'),
    ).toBeInTheDocument();
  });
});

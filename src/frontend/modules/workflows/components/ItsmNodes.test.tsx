// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { http, HttpResponse } from 'msw';
import { useTranslation } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';
import { itsmOpen } from '@nodes/itsm-open';
import { itsmStatusTrigger } from '@nodes/itsm-status-trigger';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { CommunicationChannelSelect } from './CommunicationChannelSelect';
import { NodePaletteCategorySection } from './NodePaletteCategorySection';

function Palette({ granted }: { granted: string[] }) {
  const { t } = useTranslation('common');
  const { t: tn } = useTranslation('nodes');
  return (
    <NodePaletteCategorySection
      capabilities={new Set(['channel:any', 'environment:any'])}
      permissions={{ permissions: granted, wildcard: false }}
      category="integration"
      collapsed={false}
      definitions={[itsmOpen, itsmStatusTrigger]}
      label="Integrations"
      onDragStart={() => undefined}
      onToggle={() => undefined}
      t={t}
      tn={tn}
    />
  );
}

function item(label: string) {
  return screen.getByText(label).closest('[draggable]') as HTMLElement;
}

describe('ITSM workflow nodes', () => {
  it('offers the ticket actions only with alerts.manage, naming it', () => {
    const { unmount } = renderWithProviders(<Palette granted={['workflows.manage']} />);
    expect(item('Open Ticket')).toHaveAttribute('draggable', 'false');
    expect(item('Open Ticket')).toHaveAttribute('title', 'Requires the alerts.manage permission');
    expect(item('Ticket Status')).toHaveAttribute('draggable', 'true');
    unmount();

    renderWithProviders(<Palette granted={['workflows.manage', 'alerts.manage']} />);
    expect(item('Open Ticket')).toHaveAttribute('draggable', 'true');
  });

  it('lists only ticketing channels, with no default to fall back to', async () => {
    const everyChannel = vi.fn();
    server.use(
      http.get('*/api/itsm/channels', () =>
        HttpResponse.json({
          success: true,
          data: [{ id: 'pd', name: 'PagerDuty on-call', channelType: 'pagerduty' }],
        }),
      ),
      http.get('*/api/communication-channels', () => {
        everyChannel();
        return HttpResponse.json({
          success: true,
          data: [{ id: 'sl', name: 'Slack ops', channelType: 'slack', isDefault: true, enabled: true }],
        });
      }),
    );
    const onChange = vi.fn();
    const user = userEvent.setup();
    renderWithProviders(<CommunicationChannelSelect value="" onChange={onChange} incidentOnly />);
    await user.click(screen.getByRole('button', { name: 'Select communication channel' }));
    await user.click(await screen.findByRole('button', { name: /PagerDuty on-call/ }));
    expect(onChange).toHaveBeenCalledWith('pd');
    await user.click(screen.getByRole('button', { name: 'Select communication channel' }));
    expect(screen.queryByText('Slack ops')).toBeNull();
    expect(screen.queryByText('Use default communication channel')).toBeNull();
    expect(everyChannel).not.toHaveBeenCalled();
  });
});

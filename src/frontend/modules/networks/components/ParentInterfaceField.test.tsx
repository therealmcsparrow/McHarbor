// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { http, HttpResponse } from 'msw';
import { describe, expect, it } from 'vitest';
import { renderWithProviders } from '../../../test/render';
import { server } from '../../../test/server';
import { ParentInterfaceField } from './ParentInterfaceField';

function answer(data: object) {
  server.use(http.get('*/api/networks/host-interfaces', () => HttpResponse.json({ success: true, data })));
}

describe('ParentInterfaceField', () => {
  it('blames an agent that did not answer, not a container', async () => {
    answer({ interfaces: [], agentAvailable: false, unavailable: 'agent_unreachable' });
    renderWithProviders(<ParentInterfaceField value="" onChange={() => {}} />);
    expect(await screen.findByText(/agent of this environment did not answer/)).toBeInTheDocument();
    expect(screen.queryByText(/from inside its container/)).not.toBeInTheDocument();
  });

  it('keeps the container explanation for a containerised McHarbor on a local socket', async () => {
    answer({ interfaces: [], agentAvailable: false, unavailable: 'containerized' });
    renderWithProviders(<ParentInterfaceField value="" onChange={() => {}} />);
    expect(await screen.findByText(/from inside its container/)).toBeInTheDocument();
  });
});
